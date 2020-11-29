package pointerescapes

import (
	"go/ast"
	"go/token"
	"reflect"

	"github.com/kalexmills/github-vet/cmd/vet-bot/callgraph"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer gathers a list of function signatures and indices of their pointer arguments which can be proven
// safe. A pointer argument to a function is considered safe if 1) it does not appear alone on the right-hand side
// of any assignment statement in the function body, and 2) it does not appear alone in the body of any composite
// literal.
var Analyzer = &analysis.Analyzer{
	Name:             "pointerescapes",
	Doc:              "gathers a list of function signatures and their pointer arguments which definitely do not escape during the lifetime of the function",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, callgraph.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

// Result maps function signatures to the indices of all of their safe pointer arguments.
type Result struct {
	// SafePtrs maps function signatures to a list of indices of pointer arguments which are safe.
	SafePtrs map[callgraph.Signature][]int
}

type safeArgMap map[token.Pos]map[token.Pos]int

func (sam safeArgMap) markUnsafe(funcPos token.Pos, args []ast.Expr) {
	for _, expr := range args {
		switch typed := expr.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue
			}
			delete(sam[funcPos], typed.Obj.Pos())
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			delete(sam[funcPos], id.Obj.Pos())
		}
	}
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.CompositeLit)(nil),
	}

	safeArgs := safeArgMap(make(map[token.Pos]map[token.Pos]int))

	// since lastFuncDecl is effectively global, the inspection here will attempt to remove arguments
	// that were never added to safeArgs in case of top-level CompositeLit or AssignStmts, without
	// affecting the result.
	var lastFuncDecl token.Pos
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			// all pointer args are safe until proven otherwise.
			safeArgs[n.Pos()] = parsePointerArgs(typed)
			lastFuncDecl = typed.Pos()
		case *ast.AssignStmt:
			// a pointer argument used on the RHS of an assign statement is marked unsafe.
			if _, ok := safeArgs[lastFuncDecl]; ok {
				safeArgs.markUnsafe(lastFuncDecl, typed.Rhs)
			}
		case *ast.CompositeLit:
			// a pointer argument used inside a composite literal is marked unsafe.
			if _, ok := safeArgs[lastFuncDecl]; ok {
				safeArgs.markUnsafe(lastFuncDecl, typed.Elts)
			}
		}
		return true
	})

	// construct an initial set of safe pointer arguments.
	result := Result{
		SafePtrs: make(map[callgraph.Signature][]int),
	}
	for _, decl := range graph.Calls {
		result.SafePtrs[decl.Signature] = nil
	}
	signaturesByPos := make(map[token.Pos]callgraph.SignaturePos)
	for _, sig := range graph.Signatures {
		signaturesByPos[sig.Pos] = sig
	}
	for pos, args := range safeArgs {
		sig := signaturesByPos[pos].Signature
		if _, ok := result.SafePtrs[sig]; ok {
			// take the intersection of all safe pointers; we have to do this because all our analysis
			// is based on an approximation of the call-graph.
			var argIdxs []int
			for _, arg := range args {
				argIdxs = append(argIdxs, arg)
			}
			result.SafePtrs[sig] = intersect(result.SafePtrs[sig], argIdxs)
		} else {
			for _, idx := range args {
				result.SafePtrs[sig] = append(result.SafePtrs[sig], idx)
			}
		}
	}
	// Threads the notion of an 'unsafe pointer argument' through the call-graph, by performing a breadth-first search
	// through the called-by graph, and marking unsafe caller arguments as we visit each call-site.
	callsBySignature := make(map[callgraph.Signature][]callgraph.Call)
	for _, call := range graph.Calls {
		callsBySignature[call.Signature] = append(callsBySignature[call.Signature], call)
	}
	graph.ApproxCallGraph.CalledByGraphBFS(graph.ApproxCallGraph.CalledByRoots(), func(callSig callgraph.Signature) {
		// loop over all calls with a matching signature and mark any unsafe arguments found in the signature of their caller.
		safeArgIndexes := result.SafePtrs[callSig]
		calls := callsBySignature[callSig]
		for _, call := range calls {
			for idx, argPos := range call.ArgDeclPos {
				if contains(safeArgIndexes, idx) {
					continue
				}
				// argument passed in this call is possibly unsafe, so mark the argument from the caller unsafe as well
				argIdx, ok := safeArgs[call.Caller.Pos][argPos]
				if !ok {
					continue // callerIdx can't be in SafePtrs, since SafePtrs was created from safeArgs
				}
				result.SafePtrs[call.Caller.Signature] = remove(result.SafePtrs[call.Caller.Signature], argIdx)
			}
		}
	})
	return &result, nil
}

func parsePointerArgs(n *ast.FuncDecl) map[token.Pos]int {
	result := make(map[token.Pos]int)
	argID := 0
	if n.Type.Params != nil {
		for _, x := range n.Type.Params.List {
			if _, ok := x.Type.(*ast.StarExpr); ok {
				for i := 0; i < len(x.Names); i++ {
					result[x.Names[i].Obj.Pos()] = argID
					argID++
				}
			} else {
				argID += len(x.Names)
			}
		}
	}
	return result
}

func remove(arr []int, v int) []int {
	result := arr
	for i := 0; i < len(result); i++ {
		if result[i] == v {
			result = append(result[:i], result[i+1:]...)
			i--
		}
	}
	return result
}

func contains(arr []int, v int) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}

func intersect(A, B []int) []int {
	var result []int
	for _, a := range A {
		for _, b := range B {
			if a == b {
				result = append(result, a)
				break
			}
		}
	}
	return result
}
