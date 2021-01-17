package util

import (
	"go/ast"
	"go/types"
)

// Contains returns true if arr contains x.
func Contains(arr []types.Object, x types.Object) bool {
	for _, obj := range arr {
		if x == obj {
			return true
		}
	}
	return false
}

// OutermostFuncDecl returns the source position of the outermost function declaration in  the
// provided stack.
func OutermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if fdec, ok := stack[i].(*ast.FuncDecl); ok {
			return fdec
		}
	}
	return nil
}

// FuncInputs extracts the input parameters associated with the arguments of the provided function.
func FuncInputs(info *types.Info, fdec *ast.FuncDecl) []types.Object {
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
