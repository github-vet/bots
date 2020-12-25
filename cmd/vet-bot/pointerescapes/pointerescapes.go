package pointerescapes

import (
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer gathers a list of function signatures and indices of their pointer arguments which can be proven
// safe. A pointer argument to a function is considered safe if 1) it does not appear alone on the right-hand side
// of any assignment statement in the function body, and 2) it does not appear alone in the body of any composite
// literal.
var Analyzer = &analysis.Analyzer{
	Name:             "pointerescapes",
	Doc:              "gathers a list of function signatures and their pointer arguments which definitely do not escape during the lifetime of the function",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, packid.Analyzer, callgraph.Analyzer},
	ResultType:       reflect.TypeOf((*Result)(nil)),
}

// Result maps function signatures to the indices of all of their safe pointer arguments.
type Result struct {
	// SafePtrs maps function signatures to a list of indices of pointer arguments which are safe.
	SafePtrs map[callgraph.Signature][]int
	// DangerGraph descripts the subgraph of the callgraph which consist of dangerous pointer calls.
	DangerGraph callgraph.CallGraph
	// WritesPtr is a set of signatures which were found to write a pointer.
	WritesPtr map[callgraph.Signature]struct{}
}

// pointerArgs is a map from the source position of the declaration of pointer arguments found in
// function declarations to the positional index of the argument.
type pointerArgs map[token.Pos]int

func (pa pointerArgs) Indices() []int {
	var result []int
	for _, arg := range pa {
		result = append(result, arg)
	}
	return result
}

// safeArgMap maps from the source position of function declarations to a collection of its pointer
// arguments.
type safeArgMap map[token.Pos]pointerArgs

func newSafeArgMap() safeArgMap {
	return safeArgMap(make(map[token.Pos]pointerArgs))
}

// MarkUnsafe reads the expressions provided, and removes any pointers arguments found in the provided
// list of ast.Expr from the provided function declaration, denoted by its source position. It returns
// true only if any unsafe arguments were found.
func (sam safeArgMap) MarkUnsafe(funcDeclPos token.Pos, args []ast.Expr) bool {
	anyUnsafe := false
	for _, expr := range args {
		switch typed := expr.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue
			}
			_, ok := sam[funcDeclPos][typed.Obj.Pos()]
			anyUnsafe = anyUnsafe || ok
			delete(sam[funcDeclPos], typed.Obj.Pos())
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			_, ok = sam[funcDeclPos][id.Obj.Pos()]
			anyUnsafe = anyUnsafe || ok
			delete(sam[funcDeclPos], id.Obj.Pos())
		}
	}
	return anyUnsafe
}

func run(pass *analysis.Pass) (interface{}, error) {
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	result := Result{
		SafePtrs: make(map[callgraph.Signature][]int),
	}

	safeArgs, writesPtr := inspectSafeArgs(pass)
	result.WritesPtr = writesPtr

	// handle naming collisions due to use of an approximate call-graph. We can only track
	// pointer arguments accurately when all colliding signatures share a pointer argument in
	// the same position.
	signaturesByPos := signaturesByPos(graph.Signatures)
	for pos, args := range safeArgs {
		sig := signaturesByPos[pos].Signature
		if _, ok := result.SafePtrs[sig]; ok {
			// take the intersection of all safe pointers; we have to do this because all our analysis
			// is based on an approximation of the call-graph.
			result.SafePtrs[sig] = intersect(result.SafePtrs[sig], args.Indices())
		} else {
			for _, idx := range args {
				result.SafePtrs[sig] = append(result.SafePtrs[sig], idx)
			}
		}
	}

	// add signatures for any calls whose declarations are not part of the source;
	// none of their pointer arguments are considered safe, since we can't say what the functions do.
	for _, decl := range graph.Calls {
		if _, ok := result.SafePtrs[decl.Signature]; !ok {
			result.SafePtrs[decl.Signature] = nil
		}
	}

	result.DangerGraph = callgraph.NewCallGraph()
	for sig := range writesPtr {
		result.DangerGraph.AddSignature(sig)
	}

	// Threads the notion of an 'unsafe pointer argument' through the call-graph, by performing a breadth-first search
	// through the called-by graph, and marking unsafe caller arguments as we visit each call-site.
	// In event of a naming collision, if a pointer in any of the declarations is considered unsafe, it gets marked as
	// such in all instances.
	callsBySignature := callsBySignature(graph.Calls)
	graph.ApproxCallGraph.CalledByBFS(graph.ApproxCallGraph.CalledByRoots(), func(callSig callgraph.Signature) {
		// loop over all calls with a matching signature and, if they use a pointer from their caller in an
		// unsafe position, mark the pointer from the caller unsafe also.
		safeArgIndexes := result.SafePtrs[callSig]
		calls := callsBySignature[callSig]
		for _, call := range calls {
			for idx, argPos := range call.ArgDeclPos {
				if argPos == token.NoPos || contains(safeArgIndexes, idx) {
					// argument is safe
					continue
				}
				// argument passed in this call is possibly unsafe, so mark the argument from the caller unsafe as well
				argIdx, ok := safeArgs[call.Caller.Pos][argPos]
				if !ok {
					continue // callerIdx can't be in SafePtrs, since SafePtrs was created from safeArgs
				}
				callID := result.DangerGraph.AddSignature(call.Signature)
				callerID := result.DangerGraph.AddSignature(call.Caller.Signature)
				result.DangerGraph.AddCall(callerID, callID)
				result.SafePtrs[call.Caller.Signature] = remove(result.SafePtrs[call.Caller.Signature], argIdx)
			}
		}
	})
	return &result, nil
}

// inspectSafeArgs parses the file and returns an initial safeArgMap, along with a set of
// signatures that have been found to write at least one of their pointer arguments.
func inspectSafeArgs(pass *analysis.Pass) (safeArgMap, map[callgraph.Signature]struct{}) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.CompositeLit)(nil),
	}

	safeArgs := newSafeArgMap()
	writePtrSigs := make(map[callgraph.Signature]struct{})

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			// all pointer args are safe until proven otherwise.
			safeArgs[n.Pos()] = parsePointerArgs(typed)
		case *ast.AssignStmt:
			// a pointer argument used on the RHS of an assign statement is marked unsafe.
			fdec := outermostFuncDeclPos(stack)
			if _, ok := safeArgs[fdec.Pos()]; ok {
				if safeArgs.MarkUnsafe(fdec.Pos(), typed.Rhs) {
					writePtrSigs[callgraph.SignatureFromFuncDecl(fdec)] = struct{}{}
				}
			} else {
				log.Printf("sanity check failed: assign statement found before outer declaration of %v", callgraph.SignatureFromFuncDecl(fdec))
			}
		case *ast.CompositeLit:
			// a pointer argument used inside a composite literal is marked unsafe.
			fdec := outermostFuncDeclPos(stack)
			if fdec == nil {
				continue
			}
			if _, ok := safeArgs[fdec.Pos()]; ok {
				if safeArgs.MarkUnsafe(fdec.Pos(), typed.Elts) {
					writePtrSigs[callgraph.SignatureFromFuncDecl(fdec)] = struct{}{}
				}
			} else {
				log.Printf("sanity check failed: composite literal found before outer declaration of %v", callgraph.SignatureFromFuncDecl(fdec))
			}
		}
		return true
	})
	return safeArgs, writePtrSigs
}

// outermostFuncDeclPos returns the source position of the outermost function declaration on the
// provided stack.
func outermostFuncDeclPos(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if fdec, ok := stack[i].(*ast.FuncDecl); ok {
			return fdec
		}
	}
	return nil
}

func callsBySignature(calls []callgraph.Call) map[callgraph.Signature][]callgraph.Call {
	result := make(map[callgraph.Signature][]callgraph.Call)
	for _, call := range calls {
		result[call.Signature] = append(result[call.Signature], call)
	}
	return result
}

func signaturesByPos(signatures []callgraph.SignaturePos) map[token.Pos]callgraph.SignaturePos {
	result := make(map[token.Pos]callgraph.SignaturePos)
	for _, sig := range signatures {
		result[sig.Pos] = sig
	}
	return result
}

// parsePointerArgs returns a map from the source position of all pointer arguments
// of the provided FuncDecl to the positional index of the argument in the function.
func parsePointerArgs(n *ast.FuncDecl) map[token.Pos]int {
	result := make(map[token.Pos]int)
	posIdx := 0
	if n.Type.Params != nil {
		for _, x := range n.Type.Params.List {
			if _, ok := x.Type.(*ast.StarExpr); ok {
				for i := 0; i < len(x.Names); i++ {
					result[x.Names[i].Obj.Pos()] = posIdx
					posIdx++
				}
			} else {
				posIdx += len(x.Names)
			}
		}
	}
	return result
}

func remove(arr []int, v int) []int {
	result := arr
	for i := 0; i < len(result); i++ {
		if result[i] == v {
			result = append(result[:i], result[i+1:]...)
			i--
		}
	}
	return result
}

func contains(arr []int, v int) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}

func intersect(A, B []int) []int {
	var result []int
	for _, a := range A {
		for _, b := range B {
			if a == b {
				result = append(result, a)
				break
			}
		}
	}
	return result
}
