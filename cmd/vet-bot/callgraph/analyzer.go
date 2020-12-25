package callgraph

import (
	"go/ast"
	"go/token"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/packid"
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
	Requires:         []*analysis.Analyzer{inspect.Analyzer, packid.Analyzer},
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
	// ArgDeclPos contains the source positions of the declaration of each call's arguments in order.
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

	packageResolver := pass.ResultOf[packid.Analyzer].(*packid.PackageResolver)
	result := Result{}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			sig := parseSignature(typed)
			result.Signatures = append(result.Signatures, sig)
		case *ast.CallExpr:
			if acceptlist.GlobalAcceptList != nil &&
				acceptlist.GlobalAcceptList.IgnoreCall(packageResolver, typed, stack) {
				return true
			}
			call := parseCall(typed, stack)
			result.Calls = append(result.Calls, call)
		}
		return true
	})

	cg := resultToCallGraph(result)
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
			// id.Obj == nil when the argument in the call doesn't have any 'type information'. However,
			// any argument coming straight from the function declaration will have type information
			// associated with it after parsing is complete. Since we're only interested in checking when
			// a pointer from one function is passed straight to the next; we can skip such arguments.
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
	return parseSignature(fdec).Signature
}
