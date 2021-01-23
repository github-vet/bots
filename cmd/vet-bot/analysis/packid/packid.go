package packid

import (
	"errors"
	"go/ast"
	"go/token"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:             "packid",
	Doc:              "simple package resolution to match 3rd-party functions against a list of safe calls",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf((*PackageResolver)(nil)),
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.ImportSpec)(nil),
	}

	packages := &PackageResolver{
		importsByFile: make(map[token.Pos]map[string]string),
	}

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		spec := n.(*ast.ImportSpec)
		if spec.Path == nil {
			return true // malformed import
		}

		var ident string
		packagePath := strings.ReplaceAll(spec.Path.Value, `"`, "")
		if spec.Name != nil {
			ident = strings.ReplaceAll(spec.Name.Name, `"`, "")
		} else {
			tokens := strings.Split(packagePath, "/")
			ident = tokens[len(tokens)-1]
		}

		file := outermostFile(stack)
		if _, ok := packages.importsByFile[file.Pos()]; !ok {
			packages.importsByFile[file.Pos()] = make(map[string]string)
		}
		packages.importsByFile[file.Pos()][ident] = packagePath
		return true
	})

	return packages, nil
}

type PackageResolver struct {
	importsByFile map[token.Pos]map[string]string
}

var errNotPackageCall error = errors.New("not a package call")

// PackageFor retrieves the package path for the provided call expression, if the call expression is
// a third-party function call.
func (pr *PackageResolver) PackageFor(callExpr *ast.CallExpr, stack []ast.Node) (string, error) {

	selExp, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", errNotPackageCall
	}
	x, ok := selExp.X.(*ast.Ident)
	if !ok {
		return "", errNotPackageCall
	}
	file := outermostFile(stack)
	if file == nil {
		return "", errNotPackageCall
	}
	fileImports, ok := pr.importsByFile[file.Pos()]
	if !ok {
		return "", errNotPackageCall
	}
	path, ok := fileImports[x.Name]
	if !ok {
		return "", errNotPackageCall
	}
	return path, nil
}

func outermostFile(stack []ast.Node) *ast.File {
	for i := 0; i < len(stack); i++ {
		if typed, ok := stack[i].(*ast.File); ok {
			return typed
		}
	}
	return nil
}
