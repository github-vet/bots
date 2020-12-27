package callgraph

import (
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/stats"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer provides an approximate callgraph based on function name and arity. Edges in the callgraph
// are only present if both functions are declared in the source and both take pointer arguments.
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
	// PtrSignatures contains a record for each declared function signature found to contain a pointer
	// during analysis.
	PtrSignatures []DeclaredSignature
	// PtrCalls contains a record for each function call to a function with a pointer signature found
	// during analysis.
	PtrCalls []Call
	// ApproxCallGraph contains the approximate call graph computed by the analyzer. An edge is present
	// in the callgraph between each pair of functions whose declarations are found in the source material
	// and which accept a pointer variable in their signature.
	ApproxCallGraph *CallGraph
}

// Call captures the signature of function calls along with the signature of the function from
// which they are called and position of their argument declarations.
type Call struct {
	Signature
	// Caller is the signature and position of the function which makes this call.
	Caller DeclaredSignature
	// Pos is the source position of the call.
	Pos token.Pos
	// ArgDeclPos contains the source positions of the declaration of each call's arguments in order.
	ArgDeclPos []token.Pos
}

// Signature is only an approximation of the information needed to make a function call. It captures
// only name and arity.
type Signature struct {
	Name  string
	Arity int
}

// DeclaredSignature is a signature along with the position of its declaration.
type DeclaredSignature struct {
	Signature
	Pos token.Pos
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	result := Result{}
	ptrDeclSigs := make(map[Signature]struct{}) // set of signatures with a declaration containing a pointer

	// first pass grabs all declared functions which include pointers in their signatures.
	declFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	inspect.Nodes(declFilter, func(n ast.Node, push bool) bool {
		if !push {
			return true
		}
		stats.AddCount(stats.StatFuncDecl, 1)
		decl, ok := n.(*ast.FuncDecl)
		if !ok {
			log.Fatalf("node filter %v was a lie", declFilter)
		}
		parsedSig := parseFuncDecl(decl)
		if funcDeclTakesPointers(decl) {
			ptrDeclSigs[parsedSig.Signature] = struct{}{}
			result.PtrSignatures = append(result.PtrSignatures, parsedSig)
		}
		return true
	})

	// second pass retrieves all calls which can match some declared function with a pointer in its
	// signature.
	callFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}
	inspect.WithStack(callFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		stats.AddCount(stats.StatFuncCalls, 1)
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			log.Fatalf("node filter %v was a lie", callFilter)
		}
		if _, ok := ptrDeclSigs[SignatureFromCallExpr(callExpr)]; ok {
			parsedCall := parseCallExpr(callExpr, stack)
			result.PtrCalls = append(result.PtrCalls, parsedCall)
		}
		return true
	})

	// construct the callgraph and return
	cg := resultToCallGraph(result)
	result.ApproxCallGraph = &cg
	return &result, nil
}

// parseFuncDecl retrieves a DeclaredSignature from a FuncDecl
func parseFuncDecl(fdec *ast.FuncDecl) DeclaredSignature {
	result := DeclaredSignature{Pos: fdec.Pos()}
	result.Name = fdec.Name.Name
	if fdec.Type.Params != nil {
		for _, param := range fdec.Type.Params.List {
			result.Arity += len(param.Names)
		}

	}
	return result
}

func funcDeclTakesPointers(fdec *ast.FuncDecl) bool {
	if fdec.Type.Params == nil {
		return false
	}
	result := false
	for _, param := range fdec.Type.Params.List {
		if _, ok := param.Type.(*ast.StarExpr); ok {
			result = true
		}
	}
	return result
}

// parseCallExpr retrieves relevant information about a function call, including its signature
// and the signature of the function declaration in which it appears.
func parseCallExpr(call *ast.CallExpr, stack []ast.Node) Call {
	result := Call{
		Signature: SignatureFromCallExpr(call),
		Pos:       call.Pos(),
	}
	outerFunc := outermostFuncDecl(stack)
	if outerFunc != nil {
		result.Caller = parseFuncDecl(outerFunc)
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
			// id.Obj == nil when the argument in the call doesn't have any 'type information'. However,
			// any argument coming straight from the function declaration has type information associated
			// with it after parsing is complete. Since we're only interested in checking when a pointer
			// from one function is passed straight to the next; we can skip arguments where
			// id.Obj == nil
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

// SignatureFromFuncDecl retrieves a signature from the provided FuncDecl.
func SignatureFromFuncDecl(fdec *ast.FuncDecl) Signature {
	return parseFuncDecl(fdec).Signature
}
