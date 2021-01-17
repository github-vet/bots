package facts

import (
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/typegraph"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer inductively writes facts placed by other analyzers on interesting function arguments
// throughout the callgraph, based on how the arguments are passed to other functions.
var InductFacts = &analysis.Analyzer{
	Name:             "inductfacts",
	Doc:              "inducts facts placed by other Analyzers on interesting function arguments through the callgraph",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, typegraph.Analyzer, WritesInputAnalyzer},
	ResultType:       reflect.TypeOf(InductResult{}),
}

type InductResult struct {
	Vars map[types.Object]UnsafeFacts
}

// UnsafeFacts is a bit vector representing multiple ways in which a pointer argument can be unsafe.
type UnsafeFacts int

// AFact satisfies analysis.Fact
func (*UnsafeFacts) AFact() {}

const (
	FactWritesInput UnsafeFacts = 1 << iota
	FactExternalFunc
)

func (u UnsafeFacts) String() string {
	if u == 0 {
		return "Safe"
	}
	var strs []string
	if u&FactWritesInput > 0 {
		strs = append(strs, "WritesInput")
	}
	if u&FactExternalFunc > 0 {
		strs = append(strs, "ExternalFunc")
	}

	return strings.Join(strs, "|")
}

func run(pass *analysis.Pass) (interface{}, error) {
	callsMadeByCaller := liftAllCallFactsToCaller(pass)

	inductFactsThroughCallGraph(pass, callsMadeByCaller)

	result := InductResult{
		Vars: make(map[types.Object]UnsafeFacts),
	}

	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = *fact.Fact.(*UnsafeFacts)
	}
	return result, nil
}

func inductFactsThroughCallGraph(pass *analysis.Pass, callsMadeByCaller map[*types.Func][]*ast.CallExpr) {
	cg := pass.ResultOf[typegraph.Analyzer].(*typegraph.Result)

	cg.CallGraph.CalledByBFS(cg.CallGraph.CalledByRoots(), func(caller *types.Func) {
		calls := callsMadeByCaller[caller]
		for _, call := range calls {
			callType, exported := typegraph.CallExprType(pass.TypesInfo, call)
			forEachIdent(call, func(idx int, ident *ast.Ident) {
				if callType != nil {
					liftFactsToCaller(pass, idx, pass.TypesInfo.ObjectOf(ident), callType)
				}
				if exported {
					exportExternalFuncFact(pass, pass.TypesInfo.ObjectOf(ident))
				}
			})
		}
	})
}

// liftAllCallFactsToCaller lifts analysis facts from uninteresting callsites up into the arguments
// found on the caller. This step is needed because some facts come from callsites which the
// callgraph considers uninteresting. We also collect and return a map from each function to the
// CallExpr nodes it contains.
func liftAllCallFactsToCaller(pass *analysis.Pass) map[*types.Func][]*ast.CallExpr {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	callsMadeByCaller := make(map[*types.Func][]*ast.CallExpr)
	// lift all unsafe callsite arguments up to their callers
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.CallExpr:
			fdec := outermostFuncDecl(stack)
			if fdec == nil { // top-level call
				return true
			}

			callerType := typegraph.FuncDeclType(pass.TypesInfo, fdec)
			callType, _ := typegraph.CallExprType(pass.TypesInfo, typed)

			// TODO(alex): lift facts from "uninteresting" (i.e. exported) call-sites.
			if !typegraph.InterestingSignature(callerType) || !typegraph.InterestingSignature(callType) {
				return true
			}
			// TODO(alex): ensure uniqueness for performance
			callsMadeByCaller[callerType] = append(callsMadeByCaller[callerType], typed)

		}
		return true
	})
	return callsMadeByCaller
}

func forEachIdent(callExpr *ast.CallExpr, f func(idx int, obj *ast.Ident)) {
	for idx, arg := range callExpr.Args {
		switch typed := arg.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue // argument did not come from the caller's signature.
			}
			f(idx, typed)
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident) // TODO: probably need to handle SelectorExpr here; understand implicit pointer references
			if !ok || id.Obj == nil {
				continue
			}
			f(idx, id)
		}
	}
}

// liftFactsToCaller examines the parameter from the callsite signature and lifts any facts found into the
// signature of the caller.
func liftFactsToCaller(pass *analysis.Pass, idx int, callsiteArg types.Object, call *types.Func) {
	if _, ok := callsiteArg.Type().(*types.Pointer); !ok {
		// only lift facts attached to pointer arguments
		return
	}
	// extract variable used in call
	var v *types.Var
	callSig := call.Type().(*types.Signature)
	if idx > callSig.Params().Len() && callSig.Variadic() {
		v = callSig.Params().At(callSig.Params().Len() - 1)
	} else {
		v = callSig.Params().At(idx)
	}

	// update all base-case facts stored at callsiteArg
	if _, ok := pass.ResultOf[WritesInputAnalyzer].(WritesInputResult).Vars[v]; ok {
		updateObjFact(pass, v, FactWritesInput)
	}

	// update all inductive facts stored at callsiteArg
	var vUnsafe UnsafeFacts
	pass.ImportObjectFact(v, &vUnsafe)
	if vUnsafe != 0 {
		updateObjFact(pass, callsiteArg, vUnsafe)
	}
}

func exportExternalFuncFact(pass *analysis.Pass, callsiteArg types.Object) {
	if _, ok := callsiteArg.Type().(*types.Pointer); !ok {
		return
	}
	updateObjFact(pass, callsiteArg, FactExternalFunc)
}

func updateObjFact(pass *analysis.Pass, obj types.Object, fact UnsafeFacts) {
	var export UnsafeFacts
	pass.ImportObjectFact(obj, &export)
	export |= fact
	pass.ExportObjectFact(obj, &export)
}

// InterestingSignature returns references to any interesting inputs of the provided
// function. Returns a flag if the function has interesting variadic arguments, in which
// case the last entry in result will have type Slice.
func interestingInputs(fun *types.Func) (result []types.Object, interestingVariadics bool) {
	sig, ok := fun.Type().(*types.Signature)
	if !ok {
		return
	}

	// check for pointer receiver
	v := sig.Recv()
	if v != nil {
		if _, ok := v.Type().(*types.Pointer); ok {
			result = append(result, v)
		}
	}
	// check for pointer arguments or empty interfaces
	for i := 0; i < sig.Params().Len(); i++ {
		switch typed := sig.Params().At(i).Type().(type) {
		case *types.Pointer:
			result = append(result, sig.Params().At(i))
		case *types.Interface:
			if typed.Empty() {
				result = append(result, sig.Params().At(i))
			}
		}
	}
	// handle variadic arguments
	if sig.Variadic() {
		slice, ok := sig.Params().At(sig.Params().Len() - 1).Type().(*types.Slice)
		if !ok {
			return result, false // the type-checker did something very wrong
		}
		switch typed := slice.Elem().(type) {
		case *types.Pointer:
			result = append(result, sig.Params().At(sig.Params().Len()-1))
			interestingVariadics = true
		case *types.Interface:
			if typed.Empty() {
				result = append(result, sig.Params().At(sig.Params().Len()-1))
				interestingVariadics = true
			}
		}
	}
	return
}
