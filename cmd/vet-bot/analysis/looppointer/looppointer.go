package looppointer

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/facts"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/packid"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer  checks for pointers to enclosing loop variables; modified for sweeping GitHub.
// Heavily inspired by the original looppointer: https://github.com/kyoh86/looppointer
var Analyzer = &analysis.Analyzer{
	Name:             "looppointer",
	Doc:              "checks for unary expressions to enclosing range-loop variables",
	Run:              run,
	RunDespiteErrors: true,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
		packid.Analyzer,
		facts.InductionAnalyzer},
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.RangeStmt)(nil),
		(*ast.UnaryExpr)(nil),
	}

	rangeStmts := make(map[token.Pos]*ast.RangeStmt)
	inspect.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		check(pass, n, stack, rangeStmts)
		return true
	})

	reportsByRangeStmt := make(map[*ast.RangeStmt][]analysis.Fact)
	for _, fact := range pass.AllObjectFacts() {
		if report, ok := fact.Fact.(*Report); ok {
			reportsByRangeStmt[report.RangeStmt] = append(reportsByRangeStmt[report.RangeStmt], fact.Fact)
		}
	}
	reportAll(pass, reportsByRangeStmt)
	return nil, nil
}

func check(pass *analysis.Pass, n ast.Node, stack []ast.Node, rangeStmts map[token.Pos]*ast.RangeStmt) {

	switch typed := n.(type) {
	case *ast.RangeStmt:
		if id, ok := typed.Key.(*ast.Ident); ok {
			rangeStmts[id.Pos()] = typed
		}
		if id, ok := typed.Value.(*ast.Ident); ok {
			rangeStmts[id.Pos()] = typed
		}
	case *ast.UnaryExpr:
		checkUnaryExpr(pass, typed, stack, rangeStmts)
	}
}

func checkUnaryExpr(pass *analysis.Pass, unaryExpr *ast.UnaryExpr, stack []ast.Node, rangeStmts map[token.Pos]*ast.RangeStmt) {
	if unaryExpr.Op != token.AND {
		return
	}
	id := getIdentity(unaryExpr.X)
	if id == nil || id.Obj == nil {
		return
	}
	rangeLoop, ok := rangeStmts[id.Obj.Pos()]
	if !ok {
		return
	}
	checkRangeLoopVar(pass, id, stack, rangeLoop)
}

func checkRangeLoopVar(pass *analysis.Pass, id *ast.Ident, stack []ast.Node, rangeLoop *ast.RangeStmt) {
	if n := innermostInterestingNode(stack); n != nil {
		switch typed := n.(type) {
		case *ast.CompositeLit:
			handleCompositeLit(pass, typed, id, rangeLoop)
		case *ast.AssignStmt:
			handleAssignStmt(pass, typed, id, stack, rangeLoop)
		case *ast.CallExpr:
			handleCallExpr(pass, typed, id, stack, rangeLoop)
		case *ast.BinaryExpr:
			handleBinaryExpr(pass, typed, id, rangeLoop)
		}
	}
}

func innermostInterestingNode(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		switch typed := stack[i].(type) {
		case *ast.CompositeLit:
			return typed
		case *ast.CallExpr:
			return typed
		case *ast.AssignStmt:
			return typed
		case *ast.BinaryExpr:
			return typed
		}
	}
	return nil
}

func handleBinaryExpr(pass *analysis.Pass, binExp *ast.BinaryExpr, id *ast.Ident, rangeLoop *ast.RangeStmt) {
	if binExp.Op != token.EQL && binExp.Op != token.NEQ {
		return
	}
	pass.ExportObjectFact(pass.TypesInfo.ObjectOf(id), &Report{
		InterestingNode: binExp,
		RangeStmt:       rangeLoop,
		Ident:           id,
	})
}

func handleCompositeLit(pass *analysis.Pass, compLit *ast.CompositeLit, id *ast.Ident, rangeLoop *ast.RangeStmt) {
	pass.ExportObjectFact(pass.TypesInfo.ObjectOf(id),
		&Report{
			InterestingNode: compLit,
			RangeStmt:       rangeLoop,
			Ident:           id,
		})
}

func handleAssignStmt(pass *analysis.Pass, assignment *ast.AssignStmt, id *ast.Ident, stack []ast.Node, rangeLoop *ast.RangeStmt) {
	innermostLoop := innermostLoop(stack)
	rangeLoopIsInnermost := innermostLoop == rangeLoop
	if followedBySafeBreak(stack, rangeLoopIsInnermost) {
		return
	}
	pass.ExportObjectFact(pass.TypesInfo.ObjectOf(id), &Report{
		InterestingNode: assignment,
		RangeStmt:       rangeLoop,
		Ident:           id,
	})
}

// followedBySafeBreak returns true if the current statement is followed by a return or
// a break statement which will end the loop containing the target range variable.
func followedBySafeBreak(stack []ast.Node, rangeLoopIsInnermost bool) bool {
	// TODO: is this too naive?
	var (
		innermostStmt  ast.Stmt
		innermostBlock *ast.BlockStmt
		ok             bool
	)
	for i := len(stack) - 1; i >= 0; i-- {
		if innermostStmt, ok = stack[i].(ast.Stmt); ok {
			if innermostBlock, ok = stack[i-1].(*ast.BlockStmt); ok {
				break
			} else {
				return false
			}
		}
	}
	stmtIdx := -1 // index of innermostStmt inside innermostBlock
	for idx, s := range innermostBlock.List {
		if s == innermostStmt && len(innermostBlock.List) > idx+1 {
			stmtIdx = idx + 1
			break
		}
	}
	if stmtIdx == -1 {
		return false
	}
	for i := stmtIdx; i < len(innermostBlock.List); i++ {
		switch typed := innermostBlock.List[i].(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.BranchStmt:
			return rangeLoopIsInnermost && typed.Tok == token.BREAK
		}
		break
	}
	return false
}

func handleCallExpr(pass *analysis.Pass, callExpr *ast.CallExpr, id *ast.Ident, stack []ast.Node, rangeLoop *ast.RangeStmt) {
	if acceptlist.GlobalAcceptList != nil {
		packageResolver := pass.ResultOf[packid.Analyzer].(*packid.PackageResolver)
		if acceptlist.IgnoreCall(packageResolver, callExpr, stack) {
			return
		}
	}

	inductionResult := pass.ResultOf[facts.InductionAnalyzer].(facts.InductionResult)
	callInfo, external := inductionResult.FactsForCall(pass.TypesInfo, callExpr, id)
	if external {
		pass.ExportObjectFact(pass.TypesInfo.ObjectOf(id), &Report{
			InterestingNode: callExpr,
			RangeStmt:       rangeLoop,
			Ident:           id,
		})
		return
	}
	if !callInfo.Safe() {
		pass.ExportObjectFact(pass.TypesInfo.ObjectOf(id), &UnsafeCallReport{
			Report: Report{
				InterestingNode: callExpr,
				RangeStmt:       rangeLoop,
				Ident:           id,
			},
			CallFacts: callInfo,
		})
	}
}

func innermostLoop(stack []ast.Node) *ast.RangeStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if typed, ok := stack[i].(*ast.RangeStmt); ok {
			return typed
		}
	}
	return nil
}

func getIdentity(expr ast.Expr) *ast.Ident {
	switch typed := expr.(type) {

	// we only care if an address is taken on its own  TODO: is this true??? SelectorExprs over structs may yield a pointer to a place in memory that may also change...
	case *ast.Ident:
		if typed.Obj == nil {
			return nil
		}
		return typed
	}
	return nil
}

type Report struct {
	InterestingNode ast.Node
	RangeStmt       *ast.RangeStmt
	Ident           *ast.Ident
}

func (_ *Report) AFact() {}

func (r Report) RelatedInfo(pass *analysis.Pass) analysis.RelatedInformation {
	switch r.InterestingNode.(type) {
	case *ast.AssignStmt, *ast.CompositeLit, *ast.BinaryExpr:
	default:
		log.Printf("asked for RelatedInfo on unexpected ast.Node type: %s", astutil.NodeDescription(r.InterestingNode))
		return analysis.RelatedInformation{}
	}
	return analysis.RelatedInformation{
		Pos:     r.InterestingNode.Pos(),
		End:     r.InterestingNode.End(),
		Message: r.String(),
	}
}

func (r Report) String() string {
	switch r.InterestingNode.(type) {
	case *ast.AssignStmt:
		return fmt.Sprintf("&%s used on RHS of assign statement", r.Ident.Name)
	case *ast.CompositeLit:
		return fmt.Sprintf("&%s used inside a composite literal", r.Ident.Name)
	case *ast.BinaryExpr:
		return fmt.Sprintf("&%s used in a pointer comparison", r.Ident.Name)
	default:
		return fmt.Sprintf("unexpected interesting node type %s for range-loop variable %s", astutil.NodeDescription(r.InterestingNode), r.Ident.Name)
	}
}

type UnsafeCallReport struct {
	Report
	CallFacts facts.UnsafeFacts
}

func (r UnsafeCallReport) RelatedInfo(pass *analysis.Pass) analysis.RelatedInformation {
	return analysis.RelatedInformation{
		Pos:     r.InterestingNode.Pos(),
		End:     r.InterestingNode.End(),
		Message: r.String(),
	}
}

func (r UnsafeCallReport) String() string {
	return fmt.Sprintf("variable %s passed to unsafe call; reported as: %s", r.Ident.Name, r.CallFacts.String())
}

func reportAll(pass *analysis.Pass, reportsByStmt map[*ast.RangeStmt][]analysis.Fact) {
	for stmt, facts := range reportsByStmt {
		pass.Report(analysis.Diagnostic{
			Pos:     stmt.Pos(),
			End:     stmt.End(),
			Message: "suspicious use of range-loop variable",
			Related: relatedInfo(pass, facts),
		})
	}
}

func relatedInfo(pass *analysis.Pass, facts []analysis.Fact) []analysis.RelatedInformation {
	var result []analysis.RelatedInformation
	for _, fact := range facts {
		switch typed := fact.(type) {
		case *Report:
			result = append(result, typed.RelatedInfo(pass))
		case *UnsafeCallReport:
			result = append(result, typed.RelatedInfo(pass))
		}
	}
	return result
}
