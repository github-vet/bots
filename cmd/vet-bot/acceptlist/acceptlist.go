package acceptlist

import (
	"go/ast"
	"io/ioutil"

	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"gopkg.in/yaml.v2"
)

// GlobalAcceptList stores the list of well-known third-party functions which can be ignored by analyzers.
// If the function is nil, no accept list is set.
var GlobalAcceptList *AcceptList

// AcceptList stores a list of functions from third-party packages which are known not to start
// any Goroutines or store a reference to any of their pointer arguments.
type AcceptList struct {
	Accept map[string]map[string]struct{}
}

// UnmarshalAcceptList unmarshals an AcceptList from a yaml file.
func UnmarshalAcceptList(data []byte) (AcceptList, error) {
	var unmarshaled struct {
		Accept map[string][]string
	}
	err := yaml.Unmarshal(data, &unmarshaled)
	if err != nil {
		return AcceptList{}, err
	}
	result := AcceptList{make(map[string]map[string]struct{})}
	for key, strs := range unmarshaled.Accept {
		result.Accept[key] = make(map[string]struct{})
		for _, str := range strs {
			result.Accept[key][str] = struct{}{}
		}
	}
	return result, nil
}

// AcceptListFromFile reads in the accept list from the provided file.
func AcceptListFromFile(path string) (AcceptList, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return AcceptList{}, err
	}
	result, err := UnmarshalAcceptList(data)
	if err != nil {
		return AcceptList{}, err
	}
	return result, nil
}

// IgnoreCall returns true iff the provided callExpr matches the package of a whitelisted call.
func IgnoreCall(pr *packid.PackageResolver, callExpr *ast.CallExpr, stack []ast.Node) bool {
	if GlobalAcceptList == nil || GlobalAcceptList.Accept == nil {
		return false
	}
	fun, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, err := pr.PackageFor(callExpr, stack)
	if err != nil {
		return false
	}
	acceptFuncs, ok := GlobalAcceptList.Accept[pkg]
	if !ok {
		return false
	}
	_, ok = acceptFuncs[fun.Sel.Name]
	return ok
}

// LoadAcceptList loads the accept list from the provided file path. If any errors
// occur, they are returned.
func LoadAcceptList(path string) error {
	acceptList, err := AcceptListFromFile(path)
	if err == nil {
		GlobalAcceptList = &acceptList
	}
	return err
}
