package pointerescapes

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"

	"github.com/kalexmills/github-vet/cmd/vet-bot/callgraph"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

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
	// arguments to the index of that pointer argument.
	safeArgs := make(map[token.Pos]map[*ast.Object]int)

	// detects unsafe pointer arguments to functions. An unsafe pointer argument is an argument to a function
	// which appears by itself on the RHS of an assignment statement, or are used inside a composite literal.
	var lastFuncDecl token.Pos
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
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
	signaturesByPos := make(map[token.Pos]*callgraph.SignaturePos)
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
	fmt.Println(result.SafePtrs)
	// Threads the notion of an 'unsafe pointer' back through the approximate call graph, by marking as unsafe any
	// pointer argument which is passed as an unsafe pointer to another function.
	//
	// To check this, we perform a series of breadth-first-searches through the calledBy graph, disabling all unsafe
	// arguments found along the way.
	calledByGraph := callgraph.CalledByGraph(graph.ApproxCallGraph)
	for _, call := range graph.Calls {
		bfsVisit(call.Signature, calledByGraph, func(sig callgraph.Signature) {
			safeCallArgs := result.SafePtrs[sig]
			for idx, callArg := range call.Expr.Args {
				id, ok := callArg.(*ast.Ident)
				if !ok || id.Obj == nil {
					continue
				}
				if contains(safeCallArgs, idx) { // skip if argument is known to be safe
					continue
				}
				// argument is possibly unsafe, mark the argument from the outer call unsafe as well
				outerIdx, ok := safeArgs[call.OuterSignature.Pos][id.Obj]
				if !ok {
					continue // result can't be in SafePtrs, since it was created from safeArgs
				}
				outerSig := call.OuterSignature.Signature
				result.SafePtrs[outerSig] = remove(result.SafePtrs[outerSig], outerIdx)
			}
		})
	}
	fmt.Println(result)
	return &result, nil
}

func bfsVisit(root callgraph.Signature, graph map[callgraph.Signature][]callgraph.Signature, visit func(callgraph.Signature)) {
	frontier := []callgraph.Signature{root}
	visited := make(map[callgraph.Signature]struct{})
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

func removeUsedIdents(ptrs map[*ast.Object]int, rhs []ast.Expr) map[*ast.Object]int {
	for _, expr := range rhs {
		id, ok := expr.(*ast.Ident)
		if !ok || id.Obj == nil {
			continue
		}
		delete(ptrs, id.Obj)
	}
	return ptrs
}

func parsePointerArgs(n *ast.FuncDecl) map[*ast.Object]int {
	result := make(map[*ast.Object]int)
	argID := 0
	if n.Type.Params != nil {
		for _, x := range n.Type.Params.List {
			if _, ok := x.Type.(*ast.StarExpr); ok {
				for i := 0; i < len(x.Names); i++ {
					result[x.Names[i].Obj] = argID
					argID++
				}
			} else {
				argID += len(x.Names)
			}
		}
	}
	return result
}

func innermostFuncDecl(stack []ast.Node) token.Pos {
	for i := len(stack) - 1; i <= 0; i-- {
		if _, ok := stack[i].(*ast.FuncDecl); ok {
			return stack[i].Pos()
		}
	}
	return token.NoPos
}
