package nogofunc

import (
	"go/ast"
	"go/token"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer provides a set of function signatures whose invocations definitely do not start any
// goroutines. False-positives should be expected, as no type-checking information is used during the
// analysis, which relies only on approximate knowledge of the call-graph.
var Analyzer = &analysis.Analyzer{
	Name:             "nogofunc",
	Doc:              "gathers a list of function signatures whose invocations definitely do not start a goroutine",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, packid.Analyzer, callgraph.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

// Result is a set of signatures which are guaranteed not to start any goroutines.
type Result struct {
	SyncSignatures map[callgraph.Signature]struct{}
}

type SignatureFacts struct {
	callgraph.SignaturePos
	StartsGoroutine bool
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	nodeFilter := []ast.Node{
		(*ast.GoStmt)(nil),
	}

	sigByPos := make(map[token.Pos]*SignatureFacts)
	for _, sig := range graph.Signatures {
		sigByPos[sig.Pos] = &SignatureFacts{sig, false}
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

// findSyncSignatures finds a list of Signatures for functions which do not call goroutines or call functions which
// call goroutines.
func findSyncSignatures(sigs map[token.Pos]*SignatureFacts, graph *callgraph.CallGraph) map[callgraph.Signature]struct{} {
	var toCheck []callgraph.Signature
	unsafe := make(map[callgraph.Signature]struct{})
	for _, sig := range sigs {
		if sig.StartsGoroutine {
			unsafe[sig.Signature] = struct{}{}
		} else {
			toCheck = append(toCheck, sig.Signature)
		}
	}
	// any function which calls a function that starts a goroutine is potentially unsafe. We run a BFS over the called-by
	// graph starting from the functions which start goroutines. Any function they are called by is marked as unsafe.
	unsafeNodes := make([]callgraph.Signature, 0, len(unsafe))
	for sig := range unsafe {
		unsafeNodes = append(unsafeNodes, sig)
	}
	graph.CalledByGraphBFS(unsafeNodes, func(sig callgraph.Signature) {
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
