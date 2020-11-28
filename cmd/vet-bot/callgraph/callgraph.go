package callgraph

import (
	"go/ast"
	"go/token"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:             "callgraph",
	Doc:              "computes an approximate callgraph based on function arity, name, and nothing else",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

type Result struct {
	Signatures      []*SignaturePos
	Calls           []Call
	ApproxCallGraph map[Signature][]Signature
}

// CalledByGraph reverses the provided callgraph to create the called-by graph.
func (r Result) CalledByGraph() map[Signature][]Signature {
	result := make(map[Signature][]Signature)
	for outer, callList := range r.ApproxCallGraph {
		for _, called := range callList {
			if _, ok := result[called]; ok {
				result[called] = append(result[called], outer)
			} else {
				result[called] = []Signature{outer}
			}
		}
	}
	return result
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.CallExpr)(nil),
	}

	sigByPos := make(map[token.Pos]*SignaturePos)
	result := Result{}
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push { // this is called twice, once before and after the current node is added to the stack
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			sig := parseSignature(typed)
			sigByPos[sig.Pos] = &sig
			result.Signatures = append(result.Signatures, &sig)
		case *ast.CallExpr:
			call := parseCall(typed, stack)
			result.Calls = append(result.Calls, call)
		}
		return true
	})

	result.ApproxCallGraph = makeApproxCallGraph(result)
	return &result, nil
}

// Call captures the signature of function calls.
type Call struct {
	Signature
	PtrReceiverFunc bool
	OuterSignature  SignaturePos
	Pos             token.Pos
	ArgPos          []token.Pos
}

// Signature is an approximation of the information needed to make a function call. It captures only name
// and arity.
type Signature struct {
	Name  string
	Arity int
}

// SignaturePos is a signature along with the position of its declaration.
type SignaturePos struct {
	Signature
	PtrReceiverFunc bool
	ArgPointers     []bool
	Pos             token.Pos
}

// makeApproxCallGraph constructs an approxmiate call-graph, relying on incomplete information found in the function
// signature. The resulting graph is a super-graph of the actual call-graph. That is, two functions with matching
// signatures may map to the same node in the resulting call graph, and, when they do, their edges are mapped with
// them. In other words, when the approximate call-graph and the actual call-graph are viewed as categories, they are
// related by a forgetful functor which preserves signatures (the author apologizes for this explanation ;).
func makeApproxCallGraph(r Result) map[Signature][]Signature {
	result := make(map[Signature][]Signature)
	for _, call := range r.Calls {
		outerSig := call.OuterSignature.Signature
		if _, ok := result[outerSig]; ok {
			result[outerSig] = append(result[outerSig], call.Signature)
		} else {
			result[outerSig] = []Signature{call.Signature}
		}
	}
	return result
}

// parseSignature retrieves a SignaturePos from a FuncDecl.
func parseSignature(fdec *ast.FuncDecl) SignaturePos {
	result := SignaturePos{Pos: fdec.Pos()}
	result.Name = fdec.Name.Name // we ignore _many_ things; receiver type; package, path, etc.
	if fdec.Recv != nil {
		for _, x := range fdec.Recv.List {
			if star, ok := x.Type.(*ast.StarExpr); ok {
				if _, ok := star.X.(*ast.Ident); ok {
					result.PtrReceiverFunc = true
				}
			}
		}
	}
	if fdec.Type.Params != nil {
		for _, x := range fdec.Type.Params.List {
			result.Arity += len(x.Names)
			if _, ok := x.Type.(*ast.StarExpr); ok {
				for i := 0; i < len(x.Names); i++ {
					result.ArgPointers = append(result.ArgPointers, true)
				}
			} else {
				for i := 0; i < len(x.Names); i++ {
					result.ArgPointers = append(result.ArgPointers, false)
				}
			}
		}

	}
	return result
}

// proveRootIsPointerReceiver attempts to prove that the root of a SelectorExpr has pointer type. As written, it does
// not have enough information to make that determination with certainty.
func proveRootIsPointerReceiver(selExpr *ast.SelectorExpr) bool {
	// TODO: this certainly misses some pointers; tracking the reference type of local variables could improve
	// this significantly, and seems possible since we only need to track assignments made in local scope.
	switch typed := selExpr.X.(type) {
	case *ast.Ident:
		if typed.Obj != nil && typed.Obj.Decl != nil {
			if field, ok := typed.Obj.Decl.(*ast.Field); ok {
				if _, ok := field.Type.(*ast.StarExpr); ok {
					return true
				}
			}
		}
	case *ast.SelectorExpr:
		return proveRootIsPointerReceiver(typed)
	}
	return false
}

func SignatureFromCallExpr(call *ast.CallExpr) Signature {
	result := Signature{
		Arity: len(call.Args),
	}
	switch typed := call.Fun.(type) {
	case *ast.Ident:
		result.Name = typed.Name
	case *ast.SelectorExpr:
		result.Name = typed.Sel.Name
	}
	return result
}

// parseCall retrieves relevant information about a function call.
func parseCall(call *ast.CallExpr, stack []ast.Node) Call {
	result := Call{Pos: call.Pos()}
	outerFunc := outermostFuncDecl(stack)
	if outerFunc != nil {
		result.OuterSignature = parseSignature(outerFunc)
	}
	result.Arity = len(call.Args)
	switch typed := call.Fun.(type) {
	case *ast.Ident:
		result.Name = typed.Name
		result.PtrReceiverFunc = false
	case *ast.SelectorExpr:
		result.Name = typed.Sel.Name
		result.PtrReceiverFunc = proveRootIsPointerReceiver(typed)
	}
	// obtain the source positions of argument decalarations
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || id.Obj == nil {
			result.ArgPos = append(result.ArgPos, token.NoPos)
			continue
		}
		result.ArgPos = append(result.ArgPos, id.Obj.Pos())
	}
	return result
}

func outermostFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if decl, ok := stack[i].(*ast.FuncDecl); ok {
			return decl
		}
	}
	return nil
}

// Roots returns the list of nodes which have no incoming edges.
func Roots(graph map[Signature][]Signature) []Signature {
	sigSet := make(map[Signature]struct{})
	for sig := range graph {
		sigSet[sig] = struct{}{}
	}
	for _, callers := range graph {
		for _, caller := range callers {
			delete(sigSet, caller)
		}
	}
	var result []Signature
	for sig := range sigSet {
		result = append(result, sig)
	}
	return result
}
