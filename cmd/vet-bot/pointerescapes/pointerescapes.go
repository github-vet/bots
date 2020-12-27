package pointerescapes

import (
	"go/ast"
	"go/token"
	"log"
	"reflect"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"github.com/github-vet/bots/cmd/vet-bot/stats"
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
	// ThirdPartyPtrPassed is a set of signatures which were found to pass a pointer into a third-party function.
	ThirdPartyPtrPassed map[callgraph.Signature]struct{}
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

// safePtrArgMap maps from the source position of function declarations to a collection of its pointer
// arguments.
type safePtrArgMap map[token.Pos]pointerArgs

func newSafePtrArgMap() safePtrArgMap {
	return safePtrArgMap(make(map[token.Pos]pointerArgs))
}

// MarkUnsafe reads the expressions provided, and removes any pointers arguments found in the provided
// list of ast.Expr from the provided function declaration, denoted by its source position. It returns
// true only if any pointer arguments were found and removed.
func (sam safePtrArgMap) MarkUnsafe(outerFuncDeclPos token.Pos, args []ast.Expr) bool {
	anyPointersRemoved := false
	for _, expr := range args {
		switch typed := expr.(type) {
		case *ast.Ident:
			if typed.Obj == nil {
				continue
			}
			if _, ok := sam[outerFuncDeclPos][typed.Obj.Pos()]; ok {
				anyPointersRemoved = true
				delete(sam[outerFuncDeclPos], typed.Obj.Pos())
			}
		case *ast.UnaryExpr:
			id, ok := typed.X.(*ast.Ident)
			if !ok || id.Obj == nil {
				continue
			}
			if _, ok = sam[outerFuncDeclPos][id.Obj.Pos()]; ok {
				anyPointersRemoved = true
				delete(sam[outerFuncDeclPos], id.Obj.Pos())
			}
		}
	}
	return anyPointersRemoved
}

func run(pass *analysis.Pass) (interface{}, error) {
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)

	result := Result{
		SafePtrs: make(map[callgraph.Signature][]int),
	}

	callsBySignature := callsBySignature(graph.PtrCalls)
	safePtrArgs, writesPtr, thirdPartyPtrPassed := collectSafePtrArgs(pass, callsBySignature)
	result.WritesPtr = writesPtr
	result.ThirdPartyPtrPassed = thirdPartyPtrPassed

	// handle naming collisions. We only track pointer arguments accurately when all colliding
	// signatures share a pointer argument in the same position.
	signaturesByPos := signaturesByPos(graph.PtrSignatures)
	for pos, args := range safePtrArgs {
		sig := signaturesByPos[pos].Signature
		if _, ok := result.SafePtrs[sig]; ok {
			// take the intersection of all safe pointers
			result.SafePtrs[sig] = intersect(result.SafePtrs[sig], args.Indices())
		} else {
			for _, idx := range args {
				result.SafePtrs[sig] = append(result.SafePtrs[sig], idx)
			}
		}
	}

	// keep track of the 'danger graph' -- the subgraph of the callgraph which pass
	// pointers directly to each other.
	result.DangerGraph = callgraph.NewCallGraph()
	for sig := range writesPtr {
		result.DangerGraph.AddSignature(sig)
	}

	// Threads the notion of an 'unsafe pointer argument' through the call-graph, by performing a breadth-first search
	// through the called-by graph, and marking unsafe caller arguments as we visit each call-site.
	// In event of a naming collision, if a pointer in any of the declarations is considered unsafe, it gets marked as
	// such in all instances.

	graph.ApproxCallGraph.CalledByBFS(graph.ApproxCallGraph.CalledByRoots(), func(callSig callgraph.Signature) {
		// check all calls with a matching signature and, if they use a pointer from their caller in an
		// unsafe position, mark the pointer argument in the caller unsafe also.
		safeArgIndexes := result.SafePtrs[callSig]
		calls := callsBySignature[callSig]
		for _, call := range calls {
			for idx, argPos := range call.ArgDeclPos {
				if argPos == token.NoPos || contains(safeArgIndexes, idx) {
					// argument is safe
					continue
				}
				// argument passed in this call is possibly unsafe, so mark the argument from the caller unsafe as well
				argIdx, ok := safePtrArgs[call.Caller.Pos][argPos]
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

// collectSafePtrArgs parses the file and finds all function declarations which use one of their pointer
// arguments in an assignment or a composite literal. It returns an initial safeArgMap describing the
// pointer arguments which were found to be used safelty, along with a set of signatures that have been
// found to write at least one of their pointer arguments.
func collectSafePtrArgs(pass *analysis.Pass, callsBySignature map[callgraph.Signature][]callgraph.Call) (safePtrArgs safePtrArgMap, writePtrSigs map[callgraph.Signature]struct{}, thirdPartySigs map[callgraph.Signature]struct{}) {
	graph := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result)
	// construct a set of declared functions to which a pointer is passed; any callExpr
	// containing a pointer whose signature is not found in knownPtrSignatures must be
	// third-party.
	knownPtrSignatures := make(map[callgraph.Signature]struct{})
	for _, decl := range graph.PtrSignatures {
		knownPtrSignatures[decl.Signature] = struct{}{}
	}

	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	packageResolver := pass.ResultOf[packid.Analyzer].(*packid.PackageResolver)

	safePtrArgs = newSafePtrArgMap()
	writePtrSigs = make(map[callgraph.Signature]struct{})   // declared functions which write their pointer
	thirdPartySigs = make(map[callgraph.Signature]struct{}) // declared functions which pass to third-party code

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.CompositeLit)(nil),
		(*ast.CallExpr)(nil),
	}

	visitedDeclarations := make(map[token.Pos]struct{})

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch typed := n.(type) {
		case *ast.FuncDecl:
			// all pointer args are safe until proven otherwise.
			safePtrArgs[n.Pos()] = parsePointerArgs(typed)

		// TODO: ask if this is more than a 'little' duplication
		case *ast.AssignStmt:
			// a pointer argument used on the RHS of an assign statement is marked unsafe.
			fdec := outermostFuncDeclPos(stack)
			if fdec == nil {
				return true
			}
			if _, ok := safePtrArgs[fdec.Pos()]; ok {
				if safePtrArgs.MarkUnsafe(fdec.Pos(), typed.Rhs) {
					// we found a pointer argument on the RHS of an assignment; mark the outer function.
					if _, ok := visitedDeclarations[fdec.Pos()]; !ok {
						stats.AddCount(stats.StatPtrFuncWritesPtr, 1)
					}
					writePtrSigs[callgraph.SignatureFromFuncDecl(fdec)] = struct{}{}
				}
			} else {
				log.Printf("sanity check failed: assign statement found before outer declaration of %v", callgraph.SignatureFromFuncDecl(fdec))
			}

		case *ast.CompositeLit:
			// a pointer argument used inside a composite literal is marked unsafe.
			fdec := outermostFuncDeclPos(stack)
			if fdec == nil {
				return true
			}
			if _, ok := safePtrArgs[fdec.Pos()]; !ok {
				log.Printf("sanity check failed: composite literal found before outer declaration of %v", callgraph.SignatureFromFuncDecl(fdec))
			}
			if safePtrArgs.MarkUnsafe(fdec.Pos(), typed.Elts) {
				// we found a pointer argument stored in a composite literal; mark the outer function
				if _, ok := visitedDeclarations[fdec.Pos()]; !ok {
					stats.AddCount(stats.StatPtrFuncWritesPtr, 1)
				}
				writePtrSigs[callgraph.SignatureFromFuncDecl(fdec)] = struct{}{}
			}

		case *ast.CallExpr:
			// a pointer argument passed to a third-party function is marked unsafe.
			if _, ok := knownPtrSignatures[callgraph.SignatureFromCallExpr(typed)]; ok {
				return true // if the signature is known; we found its declaration, so it can't be third-party
			}
			if acceptlist.IgnoreCall(packageResolver, typed, stack) {
				return true // ignore third-party functions in the accept list
			}
			fdec := outermostFuncDeclPos(stack)
			if fdec == nil {
				return true
			}
			if _, ok := safePtrArgs[fdec.Pos()]; !ok {
				log.Printf("sanity check failed: call expression found before outer declaration of %v", callgraph.SignatureFromFuncDecl(fdec))
				return true
			}
			if safePtrArgs.MarkUnsafe(fdec.Pos(), typed.Args) {
				// we found a pointer argument passed to this function call; mark the outer function as passing an
				// argument to third-party code.
				if _, ok := visitedDeclarations[fdec.Pos()]; !ok {
					stats.AddCount(stats.StatPtrDeclCallsThirdPartyCode, 1)
				}
				thirdPartySigs[callgraph.SignatureFromFuncDecl(fdec)] = struct{}{}
			}
		}
		return true
	})
	return safePtrArgs, writePtrSigs, thirdPartySigs
}

// outermostFuncDeclPos returns the source position of the outermost function declaration in  the
// provided stack.
func outermostFuncDeclPos(stack []ast.Node) *ast.FuncDecl {
	for i := 0; i < len(stack); i++ {
		if fdec, ok := stack[i].(*ast.FuncDecl); ok {
			return fdec
		}
	}
	return nil
}

// callsBySignature indexes a list of calls into a map keyed by signature.
func callsBySignature(calls []callgraph.Call) map[callgraph.Signature][]callgraph.Call {
	result := make(map[callgraph.Signature][]callgraph.Call)
	for _, call := range calls {
		result[call.Signature] = append(result[call.Signature], call)
	}
	return result
}

// signatureByPos indexes a list of declared signatures by their source position.
func signaturesByPos(signatures []callgraph.DeclaredSignature) map[token.Pos]callgraph.DeclaredSignature {
	result := make(map[token.Pos]callgraph.DeclaredSignature)
	for _, sig := range signatures {
		result[sig.Pos] = sig
	}
	return result
}

// parsePointerArgs returns finds the pointer arguments used in the provided FuncDecl
// and returns a map from each argument's source position to the argument's position
// within the function's list of parameters.
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

// remove removes all copies of v from arr without rearranging the array.
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
