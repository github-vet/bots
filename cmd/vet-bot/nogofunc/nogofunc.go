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

var Analyzer = &analysis.Analyzer{
	Name:             "nogofunc",
	Doc:              "gathers a list of function signatures whose invocations definitely do not start a goroutine",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, callgraph.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

type Result struct {
	SyncSignatures []callgraph.Signature
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
			sigByPos[outerFunc.Pos()].StartsGoroutine = true
		}
		return true
	})

	result.SyncSignatures = findSyncSignatures(sigByPos, graph.ApproxCallGraph)
	return &result, nil
}

type SignatureData struct {
	callgraph.SignaturePos
	StartsGoroutine bool
}

// findSyncSignatures finds a list of Signatures for functions which do not call goroutines or call functions which
// call goroutines.
func findSyncSignatures(sigs map[token.Pos]*SignatureData, graph map[callgraph.Signature][]callgraph.Signature) []callgraph.Signature {
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
	for _, sig := range toCheck {
		if bfsHitsUnsafe(sig, graph, unsafe) {
			unsafe[sig] = struct{}{}
		}
	}
	// remove unsafe signatures from the list of results
	var result []callgraph.Signature
	for _, sig := range toCheck {
		if _, ok := unsafe[sig]; !ok {
			result = append(result, sig)
		}
	}
	return result
}

// bfsHitsUnsafe determines whether the root node can reach one an unsafe node via some path in the callgraph.
func bfsHitsUnsafe(root callgraph.Signature, graph map[callgraph.Signature][]callgraph.Signature, unsafe map[callgraph.Signature]struct{}) bool {
	frontier := []callgraph.Signature{root}
	visited := make(map[callgraph.Signature]struct{})
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
