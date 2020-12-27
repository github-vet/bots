package stats

import "strings"

var statsStore statsStorage = statsStorage{
	filenames:  make(map[string]struct{}),
	countStats: make(map[CountStat]int),
}

type statsStorage struct {
	countStats map[CountStat]int
	filenames  map[string]struct{}
}

// Clear resets all storage values to zero.
func Clear() {
	statsStore.filenames = make(map[string]struct{})
	statsStore.countStats = make(map[CountStat]int)
}

// AddCount adds the provided diff to the count of the provided CountStat
func AddCount(stat CountStat, diff int) {
	statsStore.countStats[stat] += diff
}

// GetCount retrieves the current count of the provided CountStat so far.
func GetCount(stat CountStat) int {
	return statsStore.countStats[stat]
}

// AddFile counts the existence of a file and updates the values of StatFiles and StatTestFiles
func AddFile(filename string) {
	statsStore.filenames[filename] = struct{}{}
	statsStore.countStats[StatFiles]++
	if strings.HasSuffix(filename, "_test.go") {
		statsStore.countStats[StatTestFile]++
	}
	if strings.HasPrefix(filename, "vendor") {
		statsStore.countStats[StatVendoredFile]++
	}
}

// CountMissingTestFiles counts the number of files which don't have an associated test.
func CountMissingTestFiles() int {
	result := 0
	for filename := range statsStore.filenames {
		if strings.HasSuffix(filename, ".pb.go") {
			continue // we don't (ever) care about protobuf generated code
		}
		if !strings.HasSuffix(filename, "_test.go") {
			testFile := strings.TrimSuffix(filename, ".go") + "_test.go"
			if _, ok := statsStore.filenames[testFile]; !ok {
				result++
			}
		}
	}
	return result
}

type CountStat uint8

const (
	StatSloc CountStat = iota
	StatSlocTest
	StatSlocVendored
	StatFuncDecl
	StatFuncCalls
	StatRangeLoops
	StatFiles
	StatTestFile
	StatVendoredFile
	StatUnaryReferenceExpr
	StatLoopclosureHits
	StatLooppointerHits
	StatPtrFuncStartsGoroutine
	StatPtrFuncWritesPtr
	StatPtrDeclCallsThirdPartyCode
	StatLooppointerReportsWritePtr
	StatLooppointerReportsAsync
	StatLooppointerReportsThirdParty
	StatLooppointerReportsPointerReassigned
	StatLooppointerReportsCompositeLit
)

func (c CountStat) String() string {
	switch c {
	case StatSloc:
		return "StatSloc"
	case StatSlocTest:
		return "StatSlocTest"
	case StatSlocVendored:
		return "StatSlocVendored"
	case StatFuncDecl:
		return "StatFuncDecl"
	case StatFuncCalls:
		return "StatFuncCalls"
	case StatRangeLoops:
		return "StatRangeLoops"
	case StatFiles:
		return "StatFiles"
	case StatTestFile:
		return "StatTestFile"
	case StatVendoredFile:
		return "StatVendoredFile"
	case StatUnaryReferenceExpr:
		return "StatUnaryReferenceExpr"
	case StatLoopclosureHits:
		return "StatLoopclosureHits"
	case StatLooppointerHits:
		return "StatLooppointerHits"
	case StatPtrFuncStartsGoroutine:
		return "StatPtrFuncStartsGoroutine"
	case StatPtrFuncWritesPtr:
		return "StatPtrFuncWritesPtr"
	case StatPtrDeclCallsThirdPartyCode:
		return "StatPtrDeclCallsThirdPartyCode"
	case StatLooppointerReportsWritePtr:
		return "StatLooppointerReportsWritePtr"
	case StatLooppointerReportsAsync:
		return "StatLooppointerReportsAsync"
	case StatLooppointerReportsThirdParty:
		return "StatLooppointerReportsThirdParty"
	case StatLooppointerReportsPointerReassigned:
		return "StatLooppointerReportsPointerReassigned"
	case StatLooppointerReportsCompositeLit:
		return "StatLooppointerReportsCompositeLit"
	}
	return "Unknown CountStat"
}

var AllStats []CountStat = []CountStat{
	StatSloc,
	StatSlocTest,
	StatSlocVendored,
	StatFuncDecl,
	StatFuncCalls,
	StatRangeLoops,
	StatFiles,
	StatTestFile,
	StatVendoredFile,
	StatUnaryReferenceExpr,
	StatLoopclosureHits,
	StatLooppointerHits,
	StatPtrFuncStartsGoroutine,
	StatPtrFuncWritesPtr,
	StatPtrDeclCallsThirdPartyCode,
	StatLooppointerReportsWritePtr,
	StatLooppointerReportsAsync,
	StatLooppointerReportsThirdParty,
	StatLooppointerReportsPointerReassigned,
	StatLooppointerReportsCompositeLit,
}
