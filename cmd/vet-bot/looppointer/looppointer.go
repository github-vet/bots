package looppointer

import (
	"go/ast"
	"go/token"

	"github.com/kalexmills/github-vet/cmd/vet-bot/callgraph"
	"github.com/kalexmills/github-vet/cmd/vet-bot/nogofunc"
	"github.com/kalexmills/github-vet/cmd/vet-bot/pointerescapes"
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
		id, rangeLoop, digg := search.Check(n, stack, pass)
		if id != nil {
			pass.ReportRangef(rangeLoop, pass.Fset.File(n.Pos()).Name())
		}
		return digg
	})

	return nil, nil
}

type Searcher struct {
	// statement variables
	Stats map[token.Pos]*ast.RangeStmt
}

func (s *Searcher) Check(n ast.Node, stack []ast.Node, pass *analysis.Pass) (*ast.Ident, *ast.RangeStmt, bool) {
	switch typed := n.(type) {
	case *ast.RangeStmt:
		s.parseRangeStmt(typed)
	case *ast.UnaryExpr:
		return s.checkUnaryExpr(typed, stack, pass)
	}
	return nil, nil, true
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

func (s *Searcher) checkUnaryExpr(n *ast.UnaryExpr, stack []ast.Node, pass *analysis.Pass) (*ast.Ident, *ast.RangeStmt, bool) {
	if n.Op != token.AND {
		return nil, nil, true
	}

	loop := s.innermostLoop(stack)
	if loop == nil { // if this unary expression is not inside a loop, we don't even care.
		return nil, nil, true
	}

	// Get identity of the referred item
	id := getIdentity(n.X)
	if id == nil || id.Obj == nil {
		return nil, nil, true
	}

	// If the identity is not the loop statement variable,
	// it will not be reported.
	if _, isStat := s.Stats[id.Obj.Pos()]; !isStat {
		return nil, nil, true
	}
	rangeLoop := s.Stats[id.Obj.Pos()]

	// If the identity is not used in an assignment or call expression, it
	// will not be reported.
	assignStmt, child := s.assignStmt(stack)
	callExpr := s.callExpr(stack)
	if assignStmt == nil && callExpr == nil {
		return nil, nil, true
	}

	// If the identity is used in an assignStmt, it must be on the right-hand side of a '=' token by itself
	if assignStmt != nil {
		if assignStmt.Tok != token.DEFINE {
			for _, expr := range assignStmt.Rhs {
				if expr == child && child == n {
					return id, rangeLoop, false
				}
				// TODO: a common idiom seems to be to assign to an outer variable and immediately break.
				//       we can ignore these examples by examining the remainder of the block for a break statement.
			}
		}
		return nil, nil, true
	}

	// certain call expressions are safe.
	syncFuncs := pass.ResultOf[nogofunc.Analyzer].(*nogofunc.Result).SyncSignatures
	safePtrs := pass.ResultOf[pointerescapes.Analyzer].(*pointerescapes.Result).SafePtrs
	sig := callgraph.SignatureFromCallExpr(callExpr)
	if _, ok := syncFuncs[sig]; !ok {
		// TODO: report 'expect a go-routine is called'
		return id, rangeLoop, false
	}
	var callIdx int
	for idx, expr := range callExpr.Args {
		if expr == n {
			callIdx = idx
		}
	}
	for _, safeIdx := range safePtrs[sig] {
		if callIdx == safeIdx {
			return nil, nil, true
		}
	}
	// TODO: report 'expect pointer escapes'
	return id, rangeLoop, true
}

// Get variable identity
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
