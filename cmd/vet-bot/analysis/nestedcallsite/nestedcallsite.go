package nestedcallsite

import (
	"go/ast"
	"go/types"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/typegraph"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer detects callsites of functions taking pointer arguments with pointer return values whose
// arguments are immediately passed to another function. This represents another way for pointers to
// make their way around the callgraph.
var Analyzer = &analysis.Analyzer{
	Name:             "nestedcallsite",
	Doc:              "detects callsites with pointer arguments and pointer returns which immediately pass their return values to another function",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf(Result{}),
}

type Result struct {
	Vars map[types.Object]*NestedCallsite
}

type NestedCallsite struct{} // => a function variable was passed to a nested callsite

func (_ *NestedCallsite) AFact() {}

func (_ *NestedCallsite) String() string {
	return "funcNestedCallsite"
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	visited := make(map[*ast.CallExpr]struct{})

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}

		fdec := util.OutermostFuncDecl(stack)
		if fdec == nil {
			return true
		}

		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if _, ok := visited[callExpr]; ok {
			return true
		}

		var chain []*ast.CallExpr
		chain = callExprChain(callExpr, chain)
		if len(chain) <= 1 {
			return true
		}

		if !dangerousChain(pass.TypesInfo, chain) {
			return true
		}

		innermostCall := chain[len(chain)-1]
		inputs := util.FuncInputs(pass.TypesInfo, fdec)
		markUnsafe(pass, inputs, innermostCall.Args)

		// ensure we don't re-check the rest of the chain, while allowing the inspection to
		// continue into nested anonymous functions.
		for _, expr := range chain {
			visited[expr] = struct{}{}
		}
		return true
	})
	result := Result{
		Vars: make(map[types.Object]*NestedCallsite),
	}
	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = fact.Fact.(*NestedCallsite)
	}
	return result, nil
}

func markUnsafe(pass *analysis.Pass, inputs []types.Object, args []ast.Expr) {
	for _, expr := range args {
		switch typed := expr.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue
			}
			if obj := pass.TypesInfo.ObjectOf(typed); obj != nil {
				if util.Contains(inputs, obj) {
					pass.ExportObjectFact(obj, new(NestedCallsite))
				}
			}
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
				if util.Contains(inputs, obj) {
					pass.ExportObjectFact(obj, new(NestedCallsite))
				}
			}
		}
	}
}

// callExprChain retrieves a complete sequence of nested call expressions, where each
// call expression in the resulting slice provides all its arguments to its predecessor.
func callExprChain(rootExpr *ast.CallExpr, out []*ast.CallExpr) []*ast.CallExpr {
	if len(rootExpr.Args) != 1 {
		return append(out, rootExpr)
	}
	// TODO: handle qualified expressions e.g. `fmt.Println`
	// TODO: handle selector expressions resulting in a single function
	nested, ok := rootExpr.Args[0].(*ast.CallExpr)
	if !ok {
		return append(out, rootExpr)
	}
	return callExprChain(nested, append(out, rootExpr))
}

// dangerousChain returns whether a chain of nested calls is dangerous.
// A dangerous chain is one where pointers can be passed from return values into inputs
// of another function. Callgraph induction has not been written to handle this case.
// It suffices to check the two inner-most functions to see if pointer arguments can
// be passed or type-information is missing. Without more work done on callgraph
// induction, human effort will be needed in this case.
//
// TODO: more work can be done here to match the types of the innermost call with
// the types of nested calls. If they are not identical the chain is not dangerous.
func dangerousChain(info *types.Info, chain []*ast.CallExpr) bool {
	for i := len(chain) - 1; i >= len(chain)-2; i-- {
		fun, ok := info.Types[chain[i].Fun]

		if !ok {
			return true
		}
		if sig, ok := fun.Type.(*types.Signature); ok && !typegraph.InterestingSignature(sig) {
			return false
		}
	}
	return true
}
