package looppointer

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"strings"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/callgraph"
	"github.com/github-vet/bots/cmd/vet-bot/nogofunc"
	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"github.com/github-vet/bots/cmd/vet-bot/pointerescapes"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer checks for pointers to enclosing loop variables; modified for sweeping GitHub
var Analyzer = &analysis.Analyzer{
	Name:             "looppointer",
	Doc:              "checks for pointers to enclosing loop variables; modified for sweeping GitHub",
	Run:              run,
	RunDespiteErrors: true,
	Requires:         []*analysis.Analyzer{inspect.Analyzer, packid.Analyzer, callgraph.Analyzer, nogofunc.Analyzer, pointerescapes.Analyzer},
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	search := &Searcher{
		Stats: make(map[token.Pos]*ast.RangeStmt),
	}

	nodeFilter := []ast.Node{
		(*ast.RangeStmt)(nil),
		(*ast.UnaryExpr)(nil),
	}

	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		reason := search.check(n, stack, pass)
		return reason == ReasonNone // TODO: don't stop on first hit; this requires a way to aggregate multiple results into one issue description.
	})

	return nil, nil
}

// Searcher stores the set of range loops found in the source code, keyed by its
// position in the repository.
type Searcher struct {
	Stats map[token.Pos]*ast.RangeStmt
}

// Reason describes why an instance is being reported.
type Reason uint8

const (
	// ReasonNone indicates nothing is being reported.
	ReasonNone Reason = iota
	// ReasonPointerReassigned indicates a reference to a range loop variable was reassigned.
	ReasonPointerReassigned
	// ReasonCallMayWritePtr indicates some function call may store a reference to a range loop variable.
	ReasonCallMayWritePtr
	// ReasonCallMaybeAsync indicates some function call taking a reference to a range loop variable may start a Goroutine.
	ReasonCallMaybeAsync
	// ReasonCallPassesToThirdParty indicates some function call passes a pointer to third-party code
	ReasonCallPassesToThirdParty
)

// Message returns a human-readable message, provided the name of the varaible and
// its position in the source code.
func (r Reason) Message(name string, pos token.Position) string {
	switch r {
	case ReasonPointerReassigned:
		return fmt.Sprintf("reference to %s is reassigned at line %d", name, pos.Line)
	case ReasonCallMayWritePtr:
		return fmt.Sprintf("function call at line %d may store a reference to %s", pos.Line, name)
	case ReasonCallMaybeAsync:
		return fmt.Sprintf("function call which takes a reference to %s at line %d may start a goroutine", name, pos.Line)
	case ReasonCallPassesToThirdParty:
		return fmt.Sprintf("function call at line %d passes reference to %s to third-party code", pos.Line, name)
	default:
		return ""
	}
}

// TODO: passing the Reason back up is not very great.
func (s *Searcher) check(n ast.Node, stack []ast.Node, pass *analysis.Pass) Reason {
	switch typed := n.(type) {
	case *ast.RangeStmt:
		s.parseRangeStmt(typed)
	case *ast.UnaryExpr:
		return s.checkUnaryExpr(typed, stack, pass)
	}
	return ReasonNone
}

func (s *Searcher) parseRangeStmt(n *ast.RangeStmt) {
	s.addStat(n.Key, n)
	s.addStat(n.Value, n)
}

func (s *Searcher) addStat(expr ast.Expr, n *ast.RangeStmt) {
	if id, ok := expr.(*ast.Ident); ok {
		s.Stats[id.Pos()] = n
	}
}

func insertionPosition(block *ast.BlockStmt) token.Pos {
	if len(block.List) > 0 {
		return block.List[0].Pos()
	}
	return token.NoPos
}

func (s *Searcher) innermostLoop(stack []ast.Node) *ast.RangeStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.RangeStmt); ok {
			return typed
		}
	}
	return nil
}

// assignStmt returns the most recent assign statement, along with the child expression
// from the stack.
func (s *Searcher) assignStmt(stack []ast.Node) (*ast.AssignStmt, ast.Node) {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.AssignStmt); ok {
			return typed, stack[i+1]
		}
	}
	return nil, nil
}

func (s *Searcher) callExpr(stack []ast.Node) *ast.CallExpr {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.CallExpr); ok {
			return typed
		}
	}
	return nil
}

func (s *Searcher) checkUnaryExpr(n *ast.UnaryExpr, stack []ast.Node, pass *analysis.Pass) Reason {
	if n.Op != token.AND {
		return ReasonNone
	}

	innermostLoop := s.innermostLoop(stack)
	if innermostLoop == nil { // if this unary expression is not inside a loop, we don't even care.
		return ReasonNone
	}

	// Get identity of the referred item
	id := getIdentity(n.X)
	if id == nil || id.Obj == nil {
		return ReasonNone
	}

	// If the identity is not the loop statement variable,
	// it will not be reported.
	if _, isStat := s.Stats[id.Obj.Pos()]; !isStat {
		return ReasonNone
	}
	rangeLoop := s.Stats[id.Obj.Pos()]

	// If the identity is not used in an assignment or call expression, it
	// will not be reported.
	assignStmt, child := s.assignStmt(stack)
	callExpr := s.callExpr(stack)
	if assignStmt == nil && callExpr == nil {
		return ReasonNone
	}

	// TODO: encapsulate the two following cases into separate functions to make the logic here clearer.

	// If the identity is used in an AssignStmt directly, we must report that.
	if assignStmt != nil {
		// if the assignment is immediately followed by return or breaks out of the range loop, no harm can
		// be done by the assignment as the range-loop variable will never be updated again.
		innermostRangesOverID := innermostLoop == rangeLoop // true iff innermost loop ranges over the target unary expression
		if followedBySafeBreak(stack, innermostRangesOverID) {
			return ReasonNone
		}
		for _, expr := range assignStmt.Rhs {
			if expr.Pos() == child.Pos() && child.Pos() == n.Pos() {
				reportBasic(pass, rangeLoop, ReasonPointerReassigned, n, id)
				return ReasonPointerReassigned
			}
		}
		return ReasonNone
	}

	// TODO: If the identity is used in a CompositeLit directly, we must report that.

	// unaryExpr occurred in a CallExpr
	if len(callExpr.Args) == 0 {
		log.Println("sanity check failed: UnaryExpr 'occurred' inside a CallExpr with zero arguments")
	}

	// check if CallExpr is acceptlisted, ignore it if so.
	if acceptlist.GlobalAcceptList != nil {
		packageResolver := pass.ResultOf[packid.Analyzer].(*packid.PackageResolver)
		if acceptlist.IgnoreCall(packageResolver, callExpr, stack) {
			return ReasonNone
		}
	}

	// certain call expressions are safe.
	asyncFuncs := pass.ResultOf[nogofunc.Analyzer].(*nogofunc.Result).AsyncSignatures
	safePtrs := pass.ResultOf[pointerescapes.Analyzer].(*pointerescapes.Result).SafePtrs

	sig := callgraph.SignatureFromCallExpr(callExpr)

	if _, ok := asyncFuncs[sig]; ok {
		reportAsyncSuspicion(pass, rangeLoop, callExpr, id)
		return ReasonCallMaybeAsync
	}
	callIdx := -1
	for idx, expr := range callExpr.Args {
		if expr.Pos() == n.Pos() {
			callIdx = idx
			break
		}
	}
	if callIdx == -1 {
		return ReasonNone // no matching argument was found; unary Expr was nested more deeply.
	}
	for _, safeIdx := range safePtrs[sig] {
		if callIdx == safeIdx {
			return ReasonNone // we know passing a pointer in this position is safe.
		}
	}
	reportEscapedPtrSuspicion(pass, rangeLoop, callExpr, id)
	return ReasonCallMayWritePtr
}

// reportEscapedPtrSuspicion validates the suspicion and also reports when a function may allow its pointer argument
// to escape in some manner.
func reportEscapedPtrSuspicion(pass *analysis.Pass, rangeLoop *ast.RangeStmt, call *ast.CallExpr, id *ast.Ident) {
	dangerGraph := &pass.ResultOf[pointerescapes.Analyzer].(*pointerescapes.Result).DangerGraph
	writesPtr := pass.ResultOf[pointerescapes.Analyzer].(*pointerescapes.Result).WritesPtr
	thirdPartyPtrPassed := pass.ResultOf[pointerescapes.Analyzer].(*pointerescapes.Result).ThirdPartyPtrPassed

	sig := callgraph.SignatureFromCallExpr(call)
	ptrWriteGraph := make(map[string]map[string]struct{})
	thirdPartyGraph := make(map[string]map[string]struct{})

	var reason Reason
	err := dangerGraph.BFSWithStack(sig, func(sig callgraph.Signature, stack []callgraph.Signature) {
		if _, ok := writesPtr[sig]; ok {
			reason = ReasonCallMayWritePtr
			addPathToGraph(ptrWriteGraph, stack)
		}
		if _, ok := thirdPartyPtrPassed[sig]; ok {
			reason = ReasonCallPassesToThirdParty
			addPathToGraph(thirdPartyGraph, stack)
		}
	})

	var thirdPartyReport string
	if err == callgraph.ErrSignatureMissing {
		thirdPartyReport = fmt.Sprintf("root signature %v was not found in the callgraph; reference was passed directly to third-party code", sig)
	}

	report := strings.Join([]string{
		reportPathGraph(ptrWriteGraph, "function which writes a pointer argument"),
		reportPathGraph(thirdPartyGraph, "function which passes a pointer to third-party code"),
		thirdPartyReport,
	}, "\n")

	pass.Report(analysis.Diagnostic{
		Pos:     rangeLoop.Pos(),
		End:     rangeLoop.End(),
		Message: reason.Message(id.Name, pass.Fset.Position(id.Pos())),

		Related: []analysis.RelatedInformation{
			{Message: pass.Fset.File(call.Pos()).Name()},
			{Message: report},
		},
	})
}

// reportAsyncSuspicion validates the suspicion and also reports the finding that a function may lead to starting
// a goroutine.
func reportAsyncSuspicion(pass *analysis.Pass, rangeLoop *ast.RangeStmt, call *ast.CallExpr, id *ast.Ident) {
	// TODO: this also must report whenever a noted "third-party" signature is reached in the callgraph.
	startsGoroutine := pass.ResultOf[nogofunc.Analyzer].(*nogofunc.Result).ContainsGoStmt
	cg := pass.ResultOf[callgraph.Analyzer].(*callgraph.Result).ApproxCallGraph

	sig := callgraph.SignatureFromCallExpr(call)
	sigGraph := make(map[string]map[string]struct{})

	err := cg.BFSWithStack(sig, func(sig callgraph.Signature, stack []callgraph.Signature) {
		if _, ok := startsGoroutine[sig]; ok {
			addPathToGraph(sigGraph, stack)
		}
	})

	if err == callgraph.ErrSignatureMissing {
		log.Printf("async: could not find root signature %v in callgraph; suspect pointer passed directly to 3rd-party code", sig)
		// TODO?: report possible third-party code?
	}

	pass.Report(analysis.Diagnostic{
		Pos:     rangeLoop.Pos(),
		End:     rangeLoop.End(),
		Message: ReasonCallMaybeAsync.Message(id.Name, pass.Fset.Position(id.Pos())),

		Related: []analysis.RelatedInformation{
			{Message: pass.Fset.File(call.Pos()).Name()},
			{Message: reportPathGraph(sigGraph, "function calling a goroutine")},
		},
	})
}

func addPathToGraph(graph map[string]map[string]struct{}, path []callgraph.Signature) {
	for i := 0; i < len(path); i++ {
		from := fmt.Sprintf("(%s, %d)", path[i].Name, path[i].Arity)
		// put edge from -> to into graph
		fromNeighbors, ok := graph[from]
		if !ok {
			graph[from] = make(map[string]struct{})
			fromNeighbors = graph[from]
		}
		if i != len(path)-1 {
			to := fmt.Sprintf("(%s, %d)", path[i+1].Name, path[i+1].Arity)
			fromNeighbors[to] = struct{}{}
		}
	}
}

func reportPathGraph(pathGraph map[string]map[string]struct{}, badFuncPhrase string) string {
	var sb strings.Builder
	if len(pathGraph) == 0 {
		sb.WriteString("No path was found through the callgraph that could lead to a ")
		sb.WriteString(badFuncPhrase)
		sb.WriteString(".\n")
		return sb.String()
	}
	sb.WriteString("The following graphviz dot graph describes paths through the callgraph that could lead to a ")
	sb.WriteString(badFuncPhrase)
	sb.WriteString(":\n")
	sb.WriteString("digraph G {\n")
	for from, neighbors := range pathGraph {
		fmt.Fprintf(&sb, `  "%s" -> {`, from)
		for to := range neighbors {
			fmt.Fprintf(&sb, `"%s";`, to)
		}
		sb.WriteString("}\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

// TODO: remove this function and make it more specific....
func reportBasic(pass *analysis.Pass, rangeLoop *ast.RangeStmt, reason Reason, n ast.Node, id *ast.Ident) {
	pass.Report(analysis.Diagnostic{
		Pos:     rangeLoop.Pos(),
		End:     rangeLoop.End(),
		Message: reason.Message(id.Name, pass.Fset.Position(id.Pos())),

		Related: []analysis.RelatedInformation{
			{Message: pass.Fset.File(n.Pos()).Name()},
		},
	})
}

// followedBySafeBreak returns true if the current statement is followed by a return or
// a break statement which will end the loop containing the target range variable.
func followedBySafeBreak(stack []ast.Node, innermostRangesOverID bool) bool {
	stmt := innermostStmt(stack)
	block := innermostBlock(stack)
	startIdx := -1
	for idx, s := range block.List {
		if s == stmt && len(block.List) > idx+1 {
			startIdx = idx + 1
			break
		}
	}
	if startIdx == -1 {
		return false
	}
	for i := startIdx; i < len(block.List); i++ {
		switch typed := block.List[i].(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.BranchStmt:
			return innermostRangesOverID && typed.Tok == token.BREAK
		}
		break
	}
	return false
}

func innermostStmt(stack []ast.Node) ast.Stmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if stmt, ok := stack[i].(ast.Stmt); ok {
			return stmt
		}
	}
	return nil
}
func innermostBlock(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if block, ok := stack[i].(*ast.BlockStmt); ok {
			return block
		}
	}
	return nil
}

func getIdentity(expr ast.Expr) *ast.Ident {
	switch typed := expr.(type) {

	// we only care if an address is taken on its own
	case *ast.Ident:
		if typed.Obj == nil {
			return nil
		}
		return typed
	}
	return nil
}
