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
		sigByPos[sig.Pos] = &SignatureData{sig, false}
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

	result.SyncSignatures = findSyncSignatures(sigByPos, graph.ApproxCallGraph)
	return &result, nil
}

type SignatureData struct {
	callgraph.SignaturePos
	StartsGoroutine bool
}

// findSyncSignatures finds a list of Signatures for functions which do not call goroutines or call functions which
// call goroutines.
func findSyncSignatures(sigs map[token.Pos]*SignatureData, graph *callgraph.CallGraph) map[callgraph.Signature]struct{} {
	var toCheck []callgraph.Signature
	unsafe := make(map[callgraph.Signature]struct{})
	for _, sig := range sigs {
		if sig.StartsGoroutine {
			unsafe[sig.Signature] = struct{}{}
		} else {
			toCheck = append(toCheck, sig.Signature)
		}
	}
	// any function which calls a function that starts a goroutine is potentially unsafe. To find out which
	// functions could start a goroutine in the course of their execution, we run a BFS over the called-by
	// graph starting from all the unsafe nodes.
	unsafeNodes := make([]callgraph.Signature, 0, len(unsafe))
	for sig := range unsafe {
		unsafeNodes = append(unsafeNodes, sig)
	}
	graph.CalledByGraphBfs(unsafeNodes, func(sig callgraph.Signature) {
		unsafe[sig] = struct{}{}
	})
	// remove all unsafe signatures from the list of results
	result := make(map[callgraph.Signature]struct{})
	for _, sig := range toCheck {
		if _, ok := unsafe[sig]; !ok {
			result[sig] = struct{}{}
		}
	}
	return result
}

func outermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}
