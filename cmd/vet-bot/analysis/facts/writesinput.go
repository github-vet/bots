package facts

import (
	"go/ast"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer marks as unsafe all function inputs which appear within the function declaration on
// the RHS of an assignment statement or within a composite literal.
var WritesInputAnalyzer = &analysis.Analyzer{
	Name:             "writesptr",
	Doc:              "marks as unsafe all function inputs appearing on the RHS of an assignment statement or inside a composite literal",
	FactTypes:        []analysis.Fact{(*WritesInput)(nil)},
	Run:              runWritesInput,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf(WritesInputResult{}),
}

type WritesInput struct{} // => *types.Var is a function input that may be written during function

func (_ *WritesInput) AFact() {}

func (_ *WritesInput) String() string {
	return "funcWritesInput"
}

type WritesInputResult struct {
	Vars map[types.Object]*WritesInput
}

func runWritesInput(pass *analysis.Pass) (interface{}, error) {
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
			fdec := outermostFuncDecl(stack)
			if fdec != nil {
				inputs := funcInputs(pass.TypesInfo, fdec)
				markUnsafe(pass, inputs, typed.Rhs)
			}
		case *ast.CompositeLit:
			fdec := outermostFuncDecl(stack)
			if fdec != nil {
				inputs := funcInputs(pass.TypesInfo, fdec)
				markUnsafe(pass, inputs, typed.Elts)
			}
		}
		return true
	})

	result := WritesInputResult{
		Vars: make(map[types.Object]*WritesInput),
	}
	for _, fact := range pass.AllObjectFacts() {
		result.Vars[fact.Object] = fact.Fact.(*WritesInput)
	}
	return result, nil
}

// funcInputs extracts the input parameters associated with the arguments of the provided function.
func funcInputs(info *types.Info, fdec *ast.FuncDecl) []types.Object {
	fun := info.ObjectOf(fdec.Name)
	if fun == nil {
		return nil
	}
	var result []types.Object
	if fun, ok := fun.(*types.Func); ok {
		if sig, ok := fun.Type().(*types.Signature); ok {
			result = append(result, sig.Recv())
			for i := 0; i < sig.Params().Len(); i++ {
				result = append(result, sig.Params().At(i))
			}
		}
	}
	return result
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
				if contains(inputs, obj) {
					pass.ExportObjectFact(obj, new(WritesInput))
				}
			}
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			if obj := pass.TypesInfo.ObjectOf(id); obj != nil {
				if contains(inputs, obj) {
					pass.ExportObjectFact(obj, new(WritesInput))
				}
			}
		}
	}
}

// outermostFuncDecl returns the source position of the outermost function declaration in  the
// provided stack.
func outermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if fdec, ok := stack[i].(*ast.FuncDecl); ok {
			return fdec
		}
	}
	return nil
}

func contains(arr []types.Object, x types.Object) bool {
	for _, obj := range arr {
		if x == obj {
			return true
		}
	}
	return false
}
