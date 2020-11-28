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

type Result struct {
	// SafeArgs maps function signatures to a list of indices of pointer arguments which are safe.
	SafePtrs map[callgraph.Signature][]int
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.CompositeLit)(nil),
	}

	// safeArgs is a map from the position of function declarations and the defintion of their safe pointer
	// arguments to the position of the index of that pointer argument.
	safeArgs := make(map[token.Pos]map[token.Pos]int) // TODO: better encapsulate this specialized type

	// detects unsafe pointer arguments to functions. An unsafe pointer argument is an argument to a function
	// which appears by itself on the RHS of an assignment statement, or are used inside a composite literal.
	var lastFuncDecl token.Pos
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			// all pointer args are safe until proven otherwise
			safeArgs[n.Pos()] = parsePointerArgs(typed)
			lastFuncDecl = typed.Pos()
		case *ast.AssignStmt:
			// a pointer argument used on the RHS of an assign statement is marked unsafe.
			if _, ok := safeArgs[lastFuncDecl]; ok {
				safeArgs[lastFuncDecl] = removeUsedIdents(safeArgs[lastFuncDecl], typed.Rhs)
			}
		case *ast.CompositeLit:
			// a pointer argument used inside a composite literal is marked unsafe.
			if _, ok := safeArgs[lastFuncDecl]; ok {
				safeArgs[lastFuncDecl] = removeUsedIdents(safeArgs[lastFuncDecl], typed.Elts)
			}
		}
		return true
	})

	// construct the set of safe pointer arguments.
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
			// take the intersection of all safe pointers
			var argSlice []int
			for _, arg := range args {
				argSlice = append(argSlice, arg)
			}
			result.SafePtrs[sig] = intersect(result.SafePtrs[sig], argSlice)
		} else {
			for _, idx := range args {
				result.SafePtrs[sig] = append(result.SafePtrs[sig], idx)
			}
		}
	}
	// Threads the notion of an 'unsafe pointer' back through the approximate call graph, by marking as unsafe any
	// pointer argument which is passed as an unsafe pointer to another function.
	//
	// To check this, we perform a series of breadth-first-searches through the calledBy graph, disabling all unsafe
	// arguments found along the way.

	// we can do it in one BFS if we visit each call when we visit its signature.
	callsBySignature := make(map[callgraph.Signature][]callgraph.Call)
	for _, call := range graph.Calls {
		callsBySignature[call.Signature] = append(callsBySignature[call.Signature], call)
	}
	graph.ApproxCallGraph.CalledByGraphBFS(graph.ApproxCallGraph.CalledByRoots(), func(sig callgraph.Signature) {
		safeCallArgs := result.SafePtrs[sig]
		calls := callsBySignature[sig]
		for _, call := range calls {
			for idx, argPos := range call.ArgDeclPos {
				if contains(safeCallArgs, idx) { // skip if argument is known to be safe
					continue
				}
				// argument is possibly unsafe, mark the argument from the outer call unsafe as well
				outerIdx, ok := safeArgs[call.OuterSignature.Pos][argPos]
				if !ok {
					continue // result can't be in SafePtrs, since it was created from safeArgs
				}
				outerSig := call.OuterSignature.Signature
				result.SafePtrs[outerSig] = remove(result.SafePtrs[outerSig], outerIdx)
			}
		}
	})
	return &result, nil
}

func bfsVisit(roots []callgraph.Signature, graph map[callgraph.Signature][]callgraph.Signature, visit func(callgraph.Signature)) {
	frontier := make([]callgraph.Signature, 0, len(graph))
	frontier = append(frontier, roots...)
	visited := make(map[callgraph.Signature]struct{}, len(graph))
	for len(frontier) > 0 {
		curr := frontier[0]
		visited[curr] = struct{}{}
		frontier = frontier[1:]
		visit(curr)
		for _, child := range graph[curr] {
			if _, ok := visited[child]; !ok {
				frontier = append(frontier, child)
			}
		}
	}
	return
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

func removeUsedIdents(ptrs map[token.Pos]int, rhs []ast.Expr) map[token.Pos]int {
	for _, expr := range rhs {
		switch typed := expr.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue
			}
			delete(ptrs, typed.Obj.Pos())
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			delete(ptrs, id.Obj.Pos())
		}
	}
	return ptrs
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
