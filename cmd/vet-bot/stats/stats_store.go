// Package stats implements a global statistics store for instrumenting code to count the
// occurrence of important events. The stats store is not thread-safe in the slightest.
package stats

import (
	"strings"
	"sync"
)

var statsStore statsStorage = statsStorage{
	filenames:  make(map[string]struct{}),
	countStats: make(map[CountStat]int),
}

type statsStorage struct {
	countStats map[CountStat]int
	filenames  map[string]struct{}
	mut        sync.Mutex
}

// Clear resets all stores statistics to zero.
func Clear() {
	statsStore.filenames = make(map[string]struct{})
	statsStore.countStats = make(map[CountStat]int)
}

// AddCount adds the provided diff to the count of the provided CountStat
func AddCount(stat CountStat, diff int) {
	statsStore.mut.Lock()
	defer statsStore.mut.Unlock()
	statsStore.countStats[stat] += diff
}

// GetCount retrieves the current count of the provided CountStat so far.
func GetCount(stat CountStat) int {
	return statsStore.countStats[stat]
}

// AddFile counts the existence of a file and updates the values of StatFiles and StatTestFiles
func AddFile(filename string) {
	if strings.HasSuffix(filename, ".go") && !strings.HasSuffix(filename, ".pb.go") {
		statsStore.filenames[filename] = struct{}{}
		statsStore.countStats[StatFiles]++
		if strings.HasSuffix(filename, "_test.go") {
			statsStore.countStats[StatTestFile]++
		}
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
	StatFuncDecl CountStat = iota
	StatFuncCalls
	StatRangeLoops
	StatFiles
	StatTestFile
	StatUnaryReferenceExpr
	StatWritesInputHits
	StatPtrCmpHits
	StatNestedCallsiteHits
	StatAsyncCaptureHits
	StatExternalCalls
	StatReportedRangeLoops
	StatReportedRangeLoopIssues
)

func (c CountStat) String() string {
	switch c {
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
	case StatUnaryReferenceExpr:
		return "StatUnaryReferenceExpr"
	case StatWritesInputHits:
		return "StatWritesInputHits"
	case StatPtrCmpHits:
		return "StatPtrCmpHits"
	case StatNestedCallsiteHits:
		return "StatNestedCallsiteHits"
	case StatAsyncCaptureHits:
		return "StatAsyncCaptureHits"
	case StatExternalCalls:
		return "StatExternalCalls"
	case StatReportedRangeLoops:
		return "StatReportedRangeLoops"
	case StatReportedRangeLoopIssues:
		return "StatReportedRangeLoopIssues"
	}
	return "Unknown CountStat"
}

var AllStats []CountStat = []CountStat{
	StatFuncDecl,
	StatFuncCalls,
	StatRangeLoops,
	StatReportedRangeLoops,
	StatReportedRangeLoopIssues,
	StatFiles,
	StatTestFile,
	StatUnaryReferenceExpr,
	StatWritesInputHits,
	StatPtrCmpHits,
	StatNestedCallsiteHits,
	StatAsyncCaptureHits,
	StatExternalCalls, // N.B. this is append only; rearranging the stats can result in corrupted data.
}
