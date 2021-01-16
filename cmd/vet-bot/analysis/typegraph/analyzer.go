package typegraph

import (
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer provides a callgraph structure built to describe function signatures that pass interesting
// parameters, using whatever type-checking information is available.
var Analyzer = &analysis.Analyzer{
	Name:             "typegraph",
	Doc:              "computes a callgraph for the provided files, using type information available.",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

type Result struct {
	// Declarations is a map from function signatures to their declarations in the AST.
	Declarations map[*types.Func]*ast.FuncDecl
	// ExternalCalls is a set of source positions for CallExprs which do not call into declared functions.
	// Any calls in this map are not present in the callgraph.
	ExternalCalls map[token.Pos]struct{}
	// CallGraph is the callgraph produced
	CallGraph CallGraph
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	callFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.CallExpr)(nil),
	}

	result := Result{
		Declarations:  make(map[*types.Func]*ast.FuncDecl),
		ExternalCalls: make(map[token.Pos]struct{}),
		CallGraph:     NewCallGraph(),
	}
	inspect.WithStack(callFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			// add declarations to the list of signatures which have declarations.
			fun := funcDeclType(pass.TypesInfo, typed)
			result.Declarations[fun] = typed

		case *ast.CallExpr:
			// retrieve type of Expression
			call := callExprType(pass.TypesInfo, typed)
			if call == nil {
				// if no type information was found, it's a call into external code; mark it as such.
				result.ExternalCalls[typed.Pos()] = struct{}{}
				return true
			}

			if !interestingSignature(call.Type().(*types.Signature)) {
				return true
			}

			// retrieve the type of the enclosing function declaration
			caller := outermostFunc(pass.TypesInfo, stack)
			if caller == nil {
				log.Printf("FuncDecl did not have type information associated; was type-checking run?")
				return false
			}

			if !interestingSignature(caller.Type().(*types.Signature)) {
				return true
			}

			// add calls from interesting callers to the call graph.
			callID := result.CallGraph.AddFunc(call)
			callerID := result.CallGraph.AddFunc(caller)
			result.CallGraph.AddCall(callerID, callID)
		}
		return true
	})
	return &result, nil
}

// callExprType retrieves the type underlying the provided CallExpr, handling qualified
// identifiers and SelectorExpressions.
func callExprType(info *types.Info, call *ast.CallExpr) *types.Func {
	switch typed := call.Fun.(type) {
	case *ast.SelectorExpr:
		if sel, ok := info.Uses[typed.Sel]; ok {
			return sel.(*types.Func)
		} else {
			if s, ok := info.Selections[typed]; ok {
				return s.Obj().(*types.Func)
			}
			return nil
		}
	case *ast.Ident:
		return info.Uses[typed].(*types.Func)
	}
	return nil
}

// funcDeclType retrieves the type underlying the provided FuncDecl.
func funcDeclType(info *types.Info, fdec *ast.FuncDecl) *types.Func {
	return info.Defs[fdec.Name].(*types.Func)
}

// interestingSignature returns true if the provided signature has a pointer receiver,
// or an argument which is a pointer or an empty interface. Variadic arguments are supported.
func interestingSignature(sig *types.Signature) bool {
	// check for pointer receiver
	v := sig.Recv()
	if v != nil {
		if _, ok := v.Type().(*types.Pointer); ok {
			return true
		}
	}
	// check for pointer arguments or empty interfaces
	for i := 0; i < sig.Params().Len(); i++ {
		switch typed := sig.Params().At(i).Type().(type) {
		case *types.Pointer:
			return true
		case *types.Interface:
			if typed.Empty() {
				return true
			}
		}
	}
	// handle variadic arguments
	if sig.Variadic() {
		slice, ok := sig.Params().At(sig.Params().Len() - 1).Type().(*types.Slice)
		if !ok {
			return false // type-checker did something wrong
		}
		switch typed := slice.Elem().(type) {
		case *types.Pointer:
			return true
		case *types.Interface:
			if typed.Empty() {
				return true
			}
		}
	}
	return false
}

// outermostFunc returns the types.Object associated with the outermost FuncDecl in the provided stack.
func outermostFunc(info *types.Info, stack []ast.Node) *types.Func {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			if def, ok := info.Defs[decl.Name]; ok {
				return def.(*types.Func)
			}
			panic("FuncDecl did not have type information; type-checker was not run")
		}
	}
	return nil
}
