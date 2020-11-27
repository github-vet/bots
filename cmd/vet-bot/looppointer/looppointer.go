package looppointer

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:             "looppointer",
	Doc:              "checks for pointers to enclosing loop variables; modified for sweeping GitHub",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	// ResultType reflect.Type
	// FactTypes []Fact
}

func init() {
	//	Analyzer.Flags.StringVar(&v, "name", "default", "description")
}

type A int

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
		id, rangeLoop, digg := search.Check(n, stack)
		if id != nil {
			pass.ReportRangef(rangeLoop, "taking a pointer for the loop variable %s", id.Name)
		}
		return digg
	})

	return nil, nil
}

type Searcher struct {
	// statement variables
	Stats map[token.Pos]*ast.RangeStmt
}

func (s *Searcher) Check(n ast.Node, stack []ast.Node) (*ast.Ident, *ast.RangeStmt, bool) {
	switch typed := n.(type) {
	case *ast.RangeStmt:
		s.parseRangeStmt(typed)
	case *ast.UnaryExpr:
		return s.checkUnaryExpr(typed, stack)
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

func (s *Searcher) checkUnaryExpr(n *ast.UnaryExpr, stack []ast.Node) (*ast.Ident, *ast.RangeStmt, bool) {
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

	// If the identity is used in an assignStmt, it must be on the right-hand side of the '='
	if assignStmt != nil {
		for _, expr := range assignStmt.Rhs {
			if expr == child {
				return id, rangeLoop, false
			}
		}
		return nil, nil, true
	}

	return id, rangeLoop, false
}

// Get variable identity
func getIdentity(expr ast.Expr) *ast.Ident {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		// Get parent identity; i.e. `a` of the `a.b`.
		return selectorRoot(typed)

	case *ast.Ident:
		// Get simple identity; i.e. `a` of the `a`.
		if typed.Obj == nil {
			return nil
		}
		return typed
	}
	return nil
}

func selectorRoot(selector *ast.SelectorExpr) *ast.Ident {
	var exp ast.Expr = selector
	// climb up the SelectorExpr until the root is reached
	for typed, ok := exp.(*ast.SelectorExpr); ok; typed, ok = exp.(*ast.SelectorExpr) {
		exp = typed.X
	}
	if id, ok := exp.(*ast.Ident); ok {
		return id
	}
	return nil
}
