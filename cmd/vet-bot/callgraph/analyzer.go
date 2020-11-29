package callgraph

import (
	"go/ast"
	"go/token"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer provides an approximate callgraph based on function name and arity.
var Analyzer = &analysis.Analyzer{
	Name:             "callgraph",
	Doc:              "computes an approximate callgraph based on function arity, name, and nothing else",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

// Result is the result of the callgraph analyzer.
type Result struct {
	// Signatures contains a record for each declared function signature found during analysis.
	Signatures []SignaturePos
	// Calls contains a record for each function call found during analysis.
	Calls []Call
	// ApproxCallGraph contains the approximate call graph computed by the analyzer.
	ApproxCallGraph *CallGraph
}

// Call captures the signature of function calls along with the signature of the function from
// which they are called and position of their argument declarations.
type Call struct {
	Signature
	// Caller is the signature and position of the function which makes this call.
	Caller SignaturePos
	// Pos is the source position of the call.
	Pos token.Pos
	// ArgDeclPos is the source position of each call arguments's declaration.
	ArgDeclPos []token.Pos
}

// Signature is only an approximation of the information needed to make a function call. It captures
// only name and arity.
type Signature struct {
	Name  string
	Arity int
}

// SignaturePos is a signature along with the position of its declaration.
type SignaturePos struct {
	Signature
	Pos token.Pos
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.CallExpr)(nil),
	}

	sigByPos := make(map[token.Pos]*SignaturePos)
	result := Result{}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			sig := parseSignature(typed)
			sigByPos[sig.Pos] = &sig
			result.Signatures = append(result.Signatures, sig)
		case *ast.CallExpr:
			call := parseCall(typed, stack)
			result.Calls = append(result.Calls, call)
		}
		return true
	})

	cg := makeCallGraph(result)
	result.ApproxCallGraph = &cg
	return &result, nil
}

// parseSignature retrieves a SignaturePos from a FuncDecl.
func parseSignature(fdec *ast.FuncDecl) SignaturePos {
	result := SignaturePos{Pos: fdec.Pos()}
	result.Name = fdec.Name.Name
	if fdec.Type.Params != nil {
		for _, x := range fdec.Type.Params.List {
			result.Arity += len(x.Names)
		}

	}
	return result
}

// parseCall retrieves relevant information about a function call, including its signature
// and the signature of the package-level function in which it appears.
func parseCall(call *ast.CallExpr, stack []ast.Node) Call {
	result := Call{Pos: call.Pos()}
	outerFunc := outermostFuncDecl(stack)
	if outerFunc != nil {
		result.Caller = parseSignature(outerFunc)
	}
	result.Arity = len(call.Args)
	switch typed := call.Fun.(type) {
	case *ast.Ident:
		result.Name = typed.Name
	case *ast.SelectorExpr:
		result.Name = typed.Sel.Name
	}
	// obtain the source positions of argument decalarations
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || id.Obj == nil {
			result.ArgDeclPos = append(result.ArgDeclPos, token.NoPos)
			continue
		}
		result.ArgDeclPos = append(result.ArgDeclPos, id.Obj.Pos())
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

func makeCallGraph(r Result) CallGraph {
	result := CallGraph{
		signatureToId: make(map[Signature]int),
		callGraph:     make(map[int][]int),
	}
	insertSignature := func(sig Signature) int {
		id := len(result.signatures)
		result.signatures = append(result.signatures, sig)
		result.signatureToId[sig] = id
		return id
	}
	for _, call := range r.Calls {
		callerSig := call.Caller.Signature
		callerID, ok := result.signatureToId[callerSig]
		if !ok {
			callerID = insertSignature(callerSig)
		}
		callID, ok := result.signatureToId[call.Signature]
		if !ok {
			callID = insertSignature(call.Signature)
		}
		if !contains(result.callGraph[callerID], callID) {
			result.callGraph[callerID] = append(result.callGraph[callerID], callID)
		}
	}
	return result
}

// SignatureFromCallExpr retrieves the signature of a call expression.
func SignatureFromCallExpr(call *ast.CallExpr) Signature {
	result := Signature{
		Arity: len(call.Args),
	}
	switch typed := call.Fun.(type) {
	case *ast.Ident:
		result.Name = typed.Name
	case *ast.SelectorExpr:
		result.Name = typed.Sel.Name
	}
	return result
}
