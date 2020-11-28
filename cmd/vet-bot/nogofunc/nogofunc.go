package nogofunc

import (
	"go/ast"
	"go/token"
	"reflect"

	"github.com/kalexmills/github-vet/cmd/vet-bot/callgraph"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// TODO: something has a memory leak -.-
var Analyzer = &analysis.Analyzer{
	Name:             "nogofunc",
	Doc:              "gathers a list of function signatures whose invocations definitely do not start a goroutine",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, callgraph.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

type Result struct {
	SyncSignatures map[callgraph.Signature]struct{}
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	nodeFilter := []ast.Node{
		(*ast.GoStmt)(nil),
	}

	sigByPos := make(map[token.Pos]*SignatureData)
	for _, sig := range graph.Signatures {
		sigByPos[sig.Pos] = &SignatureData{*sig, false}
	}

	result := Result{}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push { // this is called twice, once before and after the current node is added to the stack
			return true
		}
		switch n.(type) {
		case *ast.GoStmt: // goroutine here could be nested inside a function literal; we count it anyway.
			outerFunc := outermostFuncDecl(stack)
			if outerFunc != nil {
				sigByPos[outerFunc.Pos()].StartsGoroutine = true
			}
		}
		return true
	})

	result.SyncSignatures = findSyncSignatures(sigByPos, graph.CalledByGraph())
	return &result, nil
}

type SignatureData struct {
	callgraph.SignaturePos
	StartsGoroutine bool
}

// findSyncSignatures finds a list of Signatures for functions which do not call goroutines or call functions which
// call goroutines.
func findSyncSignatures(sigs map[token.Pos]*SignatureData, calledByGraph map[callgraph.Signature][]callgraph.Signature) map[callgraph.Signature]struct{} {
	var toCheck []callgraph.Signature
	unsafe := make(map[callgraph.Signature]struct{})
	for _, sig := range sigs {
		if sig.StartsGoroutine {
			unsafe[sig.Signature] = struct{}{}
		} else {
			toCheck = append(toCheck, sig.Signature)
		}
	}
	// any function which calls a function that starts a goroutine is potentially unsafe
	unsafe = bfsSpreadUnsafe(unsafe, calledByGraph)
	// remove all unsafe signatures from the list of results
	result := make(map[callgraph.Signature]struct{})
	for _, sig := range toCheck {
		if _, ok := unsafe[sig]; !ok {
			result[sig] = struct{}{}
		}
	}
	return result
}

// bfsSpreadUnsafe pushes the unsafe functions through the calledByGraph via a single BFS rooted at known unsafe functions.
func bfsSpreadUnsafe(unsafe map[callgraph.Signature]struct{}, calledByGraph map[callgraph.Signature][]callgraph.Signature) map[callgraph.Signature]struct{} {
	roots := make([]callgraph.Signature, 0, len(unsafe))
	for sig := range unsafe {
		roots = append(roots, sig)
	}
	frontier := make([]callgraph.Signature, 0, len(calledByGraph))
	frontier = append(frontier, roots...)
	visited := make(map[callgraph.Signature]struct{}, len(calledByGraph))
	for len(frontier) > 0 {
		curr := frontier[0]
		visited[curr] = struct{}{}
		frontier = frontier[1:]
		unsafe[curr] = struct{}{} // mark any node reachable from an unsafe node as unsafe
		for _, child := range calledByGraph[curr] {
			if _, ok := visited[child]; !ok {
				frontier = append(frontier, child)
			}
		}
	}
	return unsafe
}

// bfsHitsUnsafe determines whether the root node can reach one an unsafe node via some path in the callgraph.
func bfsHitsUnsafe(root callgraph.Signature, graph map[callgraph.Signature][]callgraph.Signature, unsafe map[callgraph.Signature]struct{}) bool {
	frontier := make([]callgraph.Signature, 0, len(graph))
	frontier = append(frontier, root)
	visited := make(map[callgraph.Signature]struct{}, len(graph))
	for len(frontier) > 0 {
		curr := frontier[0]
		visited[curr] = struct{}{}
		frontier = frontier[1:]
		if _, ok := unsafe[curr]; ok {
			return true
		}
		for _, child := range graph[curr] {
			if _, ok := visited[child]; !ok {
				frontier = append(frontier, child)
			}
		}
	}

	return false
}

func outermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}
