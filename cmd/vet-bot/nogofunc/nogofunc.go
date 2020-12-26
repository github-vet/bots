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

// Result is the result of the nogofunc analyzer.
type Result struct {
	// SyncSignatures is a set of signature which are guaranteed not to start any goroutines.
	SyncSignatures map[callgraph.Signature]struct{}
	// ContainsGoStmt is a set of signatures whose declarations contain a go statement.
	ContainsGoStmt map[callgraph.Signature]struct{}
}

type signatureFacts struct {
	callgraph.DeclaredSignature
	StartsGoroutine bool
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	nodeFilter := []ast.Node{
		(*ast.GoStmt)(nil),
	}

	// nogofunc finds a list of functions declared in the target repository which don't start any
	// goroutines on their own. Calling into third-party code can be ignored
	sigByPos := make(map[token.Pos]*signatureFacts)
	for _, sig := range graph.PtrSignatures {
		sigByPos[sig.Pos] = &signatureFacts{sig, false}
	}

	result := Result{}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push { // this is called twice, once before and after the current node is added to the stack
			return true
		}
		switch n.(type) {
		case *ast.GoStmt: // goroutine here could be nested inside a function literal; we count it anyway.
			outerFunc := outermostFuncDecl(stack)
			if outerFunc != nil && sigByPos[outerFunc.Pos()] != nil {
				sigByPos[outerFunc.Pos()].StartsGoroutine = true
			}
		}
		return true
	})

	result.ContainsGoStmt, result.SyncSignatures = findSyncSignatures(sigByPos, graph.ApproxCallGraph)
	return &result, nil
}

// findSyncSignatures finds a list of Signatures for functions which do not call goroutines or call functions which
// call goroutines.
func findSyncSignatures(sigs map[token.Pos]*signatureFacts, graph *callgraph.CallGraph) (map[callgraph.Signature]struct{}, map[callgraph.Signature]struct{}) {
	var toCheck []callgraph.Signature
	startsGoroutine := make(map[callgraph.Signature]struct{})
	unsafe := make(map[callgraph.Signature]struct{})
	for _, sig := range sigs {
		if sig.StartsGoroutine {
			startsGoroutine[sig.Signature] = struct{}{}
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
	graph.CalledByBFS(unsafeNodes, func(sig callgraph.Signature) {
		unsafe[sig] = struct{}{}
	})
	// remove all unsafe signatures from the list of results
	syncSignatures := make(map[callgraph.Signature]struct{})
	for _, sig := range toCheck {
		if _, ok := unsafe[sig]; !ok {
			syncSignatures[sig] = struct{}{}
		}
	}
	return startsGoroutine, syncSignatures
}

func outermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}
