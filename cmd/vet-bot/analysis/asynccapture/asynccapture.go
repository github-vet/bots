package asynccapture

import (
	"go/ast"
	"go/types"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer marks as unsafe all function inputs which appear in the body of a goroutine or defer statement.
var Analyzer = &analysis.Analyzer{
	Name:             "asynccapture",
	Doc:              "marks as unsafe all function inputs which are directly referred to in goroutines or defer blocks",
	FactTypes:        []analysis.Fact{(*CapturesAsync)(nil)},
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf(Result{}),
}

type CapturesAsync struct{}

func (_ *CapturesAsync) AFact() {}

func (_ *CapturesAsync) String() string {
	return "funcAsyncCaptures"
}

type Result struct {
	Vars map[types.Object]*CapturesAsync
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.GoStmt)(nil),
		(*ast.DeferStmt)(nil),
	}

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.GoStmt:
			fdec := util.OutermostFuncDecl(stack)
			markAllIdentsUnsafe(pass, typed.Call, util.FuncInputs(pass.TypesInfo, fdec))
		case *ast.DeferStmt:
			fdec := util.OutermostFuncDecl(stack)
			markAllIdentsUnsafe(pass, typed.Call, util.FuncInputs(pass.TypesInfo, fdec))
		}
		return true
	})

	result := Result{
		Vars: make(map[types.Object]*CapturesAsync),
	}
	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = fact.Fact.(*CapturesAsync)
	}

	return result, nil
}

func markAllIdentsUnsafe(pass *analysis.Pass, n ast.Node, inputs []types.Object) {
	ast.Inspect(n, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
			if util.Contains(inputs, obj) {
				pass.ExportObjectFact(obj, new(CapturesAsync))
			}
		}
		return true
	})
}
