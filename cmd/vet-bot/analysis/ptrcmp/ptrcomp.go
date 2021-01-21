package ptrcmp

import (
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
)

var Analyzer = &analysis.Analyzer{
	Name:             "ptrcmp",
	Doc:              "marks function arguments which are directly used in comparisons",
	Run:              run,
	RunDespiteErrors: true,
	FactTypes:        []analysis.Fact{(*ComparesInput)(nil)},
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf(Result{}),
}

type Result struct {
	Vars map[types.Object]*ComparesInput
}

type ComparesInput struct{} // => *types.Var is a function input that appears on either side of a == or != expression.

func (_ *ComparesInput) AFact() {}

func (_ *ComparesInput) String() string {
	return "funcComparesInput"
}

func run(pass *analysis.Pass) (interface{}, error) {
	if pass == nil {
		log.Fatal("ack! pass was nil!")
	}
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.BinaryExpr)(nil),
	}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		exp := n.(*ast.BinaryExpr)
		if exp.Op != token.EQL && exp.Op != token.NEQ { // pointers are only comparable with '==' and '!='
			return true
		}

		fdec := util.OutermostFuncDecl(stack)
		if fdec == nil {
			return true
		}
		inputs := util.FuncInputs(pass.TypesInfo, fdec)
		markUnsafe(pass, inputs, exp.X)
		markUnsafe(pass, inputs, exp.Y)
		return true
	})

	result := Result{
		Vars: make(map[types.Object]*ComparesInput),
	}

	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = fact.Fact.(*ComparesInput)
	}

	return result, nil
}

func markUnsafe(pass *analysis.Pass, inputs []types.Object, expr ast.Expr) {
	switch typed := expr.(type) {
	case *ast.Ident:
		if typed.Obj == nil {
			return
		}
		if obj := pass.TypesInfo.ObjectOf(typed); obj != nil {
			if util.Contains(inputs, obj) {
				pass.ExportObjectFact(obj, new(ComparesInput))
			}
		}
	case *ast.UnaryExpr:
		id, ok := typed.X.(*ast.Ident)
		if !ok || id.Obj == nil {
			return
		}
		if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
			if util.Contains(inputs, obj) {
				pass.ExportObjectFact(obj, new(ComparesInput))
			}
		}
	}
}
