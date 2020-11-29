package looppointer

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
	"github.com/github-vet/bots/cmd/vet-bot/nogofunc"
	"github.com/github-vet/bots/cmd/vet-bot/pointerescapes"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:             "looppointer",
	Doc:              "checks for pointers to enclosing loop variables; modified for sweeping GitHub",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, callgraph.Analyzer, nogofunc.Analyzer, pointerescapes.Analyzer},
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	search := &Searcher{
		Stats: make(map[token.Pos]*ast.RangeStmt),
	}

	nodeFilter := []ast.Node{
		(*ast.RangeStmt)(nil),
		(*ast.UnaryExpr)(nil),
	}

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		id, rangeLoop, reason := search.Check(n, stack, pass)
		if id != nil {
			pass.Report(analysis.Diagnostic{
				Pos:     rangeLoop.Pos(),
				End:     rangeLoop.End(),
				Message: reason.Message(id.Name, pass.Fset.Position(id.Pos())),
				Related: []analysis.RelatedInformation{
					{Message: pass.Fset.File(n.Pos()).Name()},
				},
			})
		}
		return reason == ReasonNone
	})

	return nil, nil
}

type Searcher struct {
	Stats map[token.Pos]*ast.RangeStmt
}

type Reason uint8

const (
	ReasonNone Reason = iota
	ReasonPointerReassigned
	ReasonCallMayWritePtr
	ReasonCallMaybeAsync
)

func (r Reason) Message(name string, pos token.Position) string {
	switch r {
	case ReasonPointerReassigned:
		return fmt.Sprintf("reference to %s is reassigned at line %d", name, pos.Line)
	case ReasonCallMayWritePtr:
		return fmt.Sprintf("function call at line %d may store a reference to %s", pos.Line, name)
	case ReasonCallMaybeAsync:
		return fmt.Sprintf("function call which takes a reference to %s at line %d may start a goroutine", name, pos.Line)
	default:
		return ""
	}
}

func (s *Searcher) Check(n ast.Node, stack []ast.Node, pass *analysis.Pass) (*ast.Ident, *ast.RangeStmt, Reason) {
	switch typed := n.(type) {
	case *ast.RangeStmt:
		s.parseRangeStmt(typed)
	case *ast.UnaryExpr:
		return s.checkUnaryExpr(typed, stack, pass)
	}
	return nil, nil, ReasonNone
}

func (s *Searcher) parseRangeStmt(n *ast.RangeStmt) {
	s.addStat(n.Key, n)
	s.addStat(n.Value, n)
}

func (s *Searcher) addStat(expr ast.Expr, n *ast.RangeStmt) {
	if id, ok := expr.(*ast.Ident); ok {
		s.Stats[id.Pos()] = n
	}
}

func insertionPosition(block *ast.BlockStmt) token.Pos {
	if len(block.List) > 0 {
		return block.List[0].Pos()
	}
	return token.NoPos
}

func (s *Searcher) innermostLoop(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.RangeStmt); ok {
			return typed
		}
	}
	return nil
}

// assignStmt returns the most recent assign statement, along with the child expression
// from the stack.
func (s *Searcher) assignStmt(stack []ast.Node) (*ast.AssignStmt, ast.Node) {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.AssignStmt); ok {
			return typed, stack[i+1]
		}
	}
	return nil, nil
}

func (s *Searcher) callExpr(stack []ast.Node) *ast.CallExpr {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.CallExpr); ok {
			return typed
		}
	}
	return nil
}

func (s *Searcher) checkUnaryExpr(n *ast.UnaryExpr, stack []ast.Node, pass *analysis.Pass) (*ast.Ident, *ast.RangeStmt, Reason) {
	if n.Op != token.AND {
		return nil, nil, ReasonNone
	}

	loop := s.innermostLoop(stack)
	if loop == nil { // if this unary expression is not inside a loop, we don't even care.
		return nil, nil, ReasonNone
	}

	// Get identity of the referred item
	id := getIdentity(n.X)
	if id == nil || id.Obj == nil {
		return nil, nil, ReasonNone
	}

	// If the identity is not the loop statement variable,
	// it will not be reported.
	if _, isStat := s.Stats[id.Obj.Pos()]; !isStat {
		return nil, nil, ReasonNone
	}
	rangeLoop := s.Stats[id.Obj.Pos()]

	// If the identity is not used in an assignment or call expression, it
	// will not be reported.
	assignStmt, child := s.assignStmt(stack)
	callExpr := s.callExpr(stack)
	if assignStmt == nil && callExpr == nil {
		return nil, nil, ReasonNone
	}

	// If the identity is used in an AssignStmt, it must be on the right-hand side of a '=' token by itself
	if assignStmt != nil {
		// if the assignment is immediately followed by return or break, no harm can be done by the assignment
		// as the range-loop variable will never be updated again.
		if followedByBreaklike(stack) {
			return nil, nil, ReasonNone
		}
		if assignStmt.Tok != token.DEFINE {
			for _, expr := range assignStmt.Rhs {
				if expr == child && child == n {
					return id, rangeLoop, ReasonPointerReassigned
				}
			}
		}
		return nil, nil, ReasonNone
	}

	// unaryExpr occurred in a CallExpr

	// certain call expressions are safe.
	syncFuncs := pass.ResultOf[nogofunc.Analyzer].(*nogofunc.Result).SyncSignatures
	safePtrs := pass.ResultOf[pointerescapes.Analyzer].(*pointerescapes.Result).SafePtrs
	sig := callgraph.SignatureFromCallExpr(callExpr)
	if _, ok := syncFuncs[sig]; !ok {
		return id, rangeLoop, ReasonCallMaybeAsync
	}
	callIdx := -1
	for idx, expr := range callExpr.Args {
		if expr == n {
			callIdx = idx
			break
		}
	}
	if callIdx == -1 {
		return nil, nil, ReasonNone
	}
	for _, safeIdx := range safePtrs[sig] {
		if callIdx == safeIdx {
			return nil, nil, ReasonNone
		}
	}
	return id, rangeLoop, ReasonCallMayWritePtr
}

// followedByBreaklike returns true if the current statement is followed by a break or a
// return in the same block.
func followedByBreaklike(stack []ast.Node) bool {
	stmt := innermostStmt(stack)
	block := innermostBlock(stack)
	startIdx := -1
	for idx, s := range block.List {
		if s == stmt && len(block.List) > idx+1 {
			startIdx = idx + 1
			break
		}
	}
	if startIdx == -1 {
		return false
	}
	for i := startIdx; i < len(block.List); i++ {
		switch typed := block.List[i].(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.BranchStmt:
			return typed.Tok == token.BREAK
		}
		break
	}
	return false
}

func innermostStmt(stack []ast.Node) ast.Stmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if stmt, ok := stack[i].(ast.Stmt); ok {
			return stmt
		}
	}
	return nil
}
func innermostBlock(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if block, ok := stack[i].(*ast.BlockStmt); ok {
			return block
		}
	}
	return nil
}

func getIdentity(expr ast.Expr) *ast.Ident {
	switch typed := expr.(type) {

	// we only care if an address is taken on its own
	case *ast.Ident:
		if typed.Obj == nil {
			return nil
		}
		return typed
	}
	return nil
}
