package facts

import (
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/asynccapture"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/nestedcallsite"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/ptrcmp"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/typegraph"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/writesinput"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// InductionAnalyzer inductively writes facts placed by other analyzers on interesting function arguments
// throughout the callgraph, based on how pointer arguments are passed to other functions.
var InductionAnalyzer = &analysis.Analyzer{
	Name:             "inductfacts",
	Doc:              "inducts facts placed by other Analyzers on interesting function arguments through the callgraph",
	Run:              run,
	RunDespiteErrors: true,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
		typegraph.Analyzer,
		ptrcmp.Analyzer,
		writesinput.Analyzer,
		asynccapture.Analyzer,
		nestedcallsite.Analyzer},
	ResultType: reflect.TypeOf(InductionResult{}),
}

type InductionResult struct {
	Vars map[types.Object]UnsafeFacts
}

func (ir InductionResult) FactsForCall(info *types.Info, callExpr *ast.CallExpr, id *ast.Ident) (facts UnsafeFacts, external bool) {
	call, external := typegraph.CallExprType(info, callExpr)
	forEachIdent(callExpr, func(idx int, callIdent *ast.Ident) bool {
		if callIdent == id {
			facts = ir.Vars[callVar(idx, call)]
			return false
		}
		return true
	})
	return facts, external
}

// UnsafeFacts is a bit vector that captures multiple ways in which a pointer argument can be unsafe.
type UnsafeFacts int

// AFact satisfies analysis.Fact
func (*UnsafeFacts) AFact() {}

func (u UnsafeFacts) Safe() bool {
	return u == 0
}

const (
	FactWritesInput UnsafeFacts = 1 << iota
	FactExternalFunc
	FactComparesPtr
	FactCapturesAsync
	FactNestedCallsite
)

func (u UnsafeFacts) String() string {
	if u.Safe() {
		return "Safe"
	}
	var strs []string
	if u&FactWritesInput > 0 {
		strs = append(strs, "WritesInput")
	}
	if u&FactExternalFunc > 0 {
		strs = append(strs, "ExternalFunc")
	}
	if u&FactComparesPtr > 0 {
		strs = append(strs, "ComparesPtr")
	}
	if u&FactCapturesAsync > 0 {
		strs = append(strs, "CapturesAsync")
	}
	if u&FactNestedCallsite > 0 {
		strs = append(strs, "NestedCallsite")
	}

	return strings.Join(strs, "|")
}

func run(pass *analysis.Pass) (interface{}, error) {
	callsMadeByCaller := extractCallSites(pass)

	inductFactsThroughCallGraph(pass, callsMadeByCaller)

	result := InductionResult{
		Vars: make(map[types.Object]UnsafeFacts),
	}

	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = *fact.Fact.(*UnsafeFacts)
	}
	return result, nil
}

// inductFactsThroughCallGraph performs a BFS over the callgraph and inductively lifts facts from pointer
// arguments passed to calls inside the function declaration.
func inductFactsThroughCallGraph(pass *analysis.Pass, callsMadeByCaller map[*types.Func][]*ast.CallExpr) {
	cg := pass.ResultOf[typegraph.Analyzer].(*typegraph.Result)

	cg.CallGraph.CalledByBFS(cg.CallGraph.CalledByRoots(), func(caller *types.Func) {
		calls := callsMadeByCaller[caller]
		for _, call := range calls {
			callType, exported := typegraph.CallExprType(pass.TypesInfo, call)
			forEachIdent(call, func(idx int, ident *ast.Ident) bool {
				if callType != nil {
					liftFactsToCaller(pass, pass.TypesInfo.ObjectOf(ident), callVar(idx, callType))
				}
				if exported {
					exportExternalFuncFact(pass, pass.TypesInfo.ObjectOf(ident))
				}
				return true
			})
		}
	})
}

// extractCallSites collect and return a map from each function to all of the CallExpr nodes
// contained in their declaration. Only functions with interesting signatures are considered.
func extractCallSites(pass *analysis.Pass) map[*types.Func][]*ast.CallExpr {
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
			fdec := util.OutermostFuncDecl(stack)
			if fdec == nil { // top-level call
				return true
			}

			callerType := typegraph.FuncDeclType(pass.TypesInfo, fdec)
			callType, _ := typegraph.CallExprType(pass.TypesInfo, typed)

			if !typegraph.InterestingFunc(callerType) || !typegraph.InterestingFunc(callType) {
				return true
			}
			// TODO(alex): ensure uniqueness for performance
			callsMadeByCaller[callerType] = append(callsMadeByCaller[callerType], typed)
		}
		return true
	})
	return callsMadeByCaller
}

// forEachIdent calls the provided function for each ast.Ident expression in the arguments of
// the provided callExpr.
func forEachIdent(callExpr *ast.CallExpr, f func(idx int, obj *ast.Ident) bool) {
	for idx, arg := range callExpr.Args {
		switch typed := arg.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue // argument did not come from the caller's signature.
			}
			if !f(idx, typed) {
				return
			}
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident) // TODO: probably need to handle SelectorExpr here; understand implicit pointer references
			if !ok || id.Obj == nil {
				continue
			}
			if !f(idx, id) {
				return
			}
		}
	}
}

// liftFactsToCaller examines the parameter from the callsite signature and lifts any facts found into the
// signature of the caller.
func liftFactsToCaller(pass *analysis.Pass, callsiteArg types.Object, v *types.Var) {
	if _, ok := callsiteArg.Type().(*types.Pointer); !ok {
		// only lift facts attached to pointer arguments
		return
	}

	// update all base-case facts stored at callsiteArg
	if _, ok := pass.ResultOf[writesinput.Analyzer].(writesinput.Result).Vars[v]; ok {
		updateObjFact(pass, v, FactWritesInput)
	}

	if _, ok := pass.ResultOf[ptrcmp.Analyzer].(ptrcmp.Result).Vars[v]; ok {
		updateObjFact(pass, v, FactComparesPtr)
	}

	if _, ok := pass.ResultOf[asynccapture.Analyzer].(asynccapture.Result).Vars[v]; ok {
		updateObjFact(pass, v, FactCapturesAsync)
	}

	if _, ok := pass.ResultOf[nestedcallsite.Analyzer].(nestedcallsite.Result).Vars[v]; ok {
		updateObjFact(pass, v, FactNestedCallsite)
	}

	// update all inductive facts stored at callsiteArg
	var vUnsafe UnsafeFacts
	pass.ImportObjectFact(v, &vUnsafe)
	if vUnsafe != 0 {
		updateObjFact(pass, callsiteArg, vUnsafe)
	}
}

// callVar extracts the variable used at the given index of the provided call.
func callVar(idx int, call *types.Func) *types.Var {
	callSig := call.Type().(*types.Signature)
	if idx >= callSig.Params().Len() && callSig.Variadic() {
		return callSig.Params().At(callSig.Params().Len() - 1)
	} else {
		return callSig.Params().At(idx)
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
