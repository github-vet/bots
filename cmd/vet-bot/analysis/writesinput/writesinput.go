package writesinput

import (
	"go/ast"
	"go/types"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer marks as unsafe all function inputs which appear within the function declaration on
// the RHS of an assignment statement or within a composite literal.
var Analyzer = &analysis.Analyzer{
	Name:             "writesptr",
	Doc:              "marks as unsafe all function inputs appearing on the RHS of an assignment statement or inside a composite literal",
	FactTypes:        []analysis.Fact{(*WritesInput)(nil)},
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf(Result{}),
}

type WritesInput struct{} // => *types.Var is a function input that may be written during function

func (_ *WritesInput) AFact() {}

func (_ *WritesInput) String() string {
	return "funcWritesInput"
}

type Result struct {
	Vars map[types.Object]*WritesInput
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
		(*ast.CompositeLit)(nil),
	}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.AssignStmt:
			fdec := util.OutermostFuncDecl(stack)
			if fdec != nil {
				markUnsafe(pass, util.FuncInputs(pass.TypesInfo, fdec), typed.Rhs)
			}
		case *ast.CompositeLit:
			fdec := util.OutermostFuncDecl(stack)
			if fdec != nil {
				markUnsafe(pass, util.FuncInputs(pass.TypesInfo, fdec), typed.Elts)
			}
		}
		return true
	})

	result := Result{
		Vars: make(map[types.Object]*WritesInput),
	}
	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = fact.Fact.(*WritesInput)
	}
	return result, nil
}

// markUnsafe marks as unsafe all identifiers appearing in the provided expression array.
func markUnsafe(pass *analysis.Pass, inputs []types.Object, args []ast.Expr) {
	for _, expr := range args {
		switch typed := expr.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue
			}
			if obj := pass.TypesInfo.ObjectOf(typed); obj != nil {
				if util.Contains(inputs, obj) {
					pass.ExportObjectFact(obj, new(WritesInput))
				}
			}
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
				if util.Contains(inputs, obj) {
					pass.ExportObjectFact(obj, new(WritesInput))
				}
			}
		}
	}
}
