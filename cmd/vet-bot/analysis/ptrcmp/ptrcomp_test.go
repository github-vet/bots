package ptrcmp_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/ptrcmp"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPtrComp_BasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, ptrcmp.Analyzer, "ptrcomp_basic")
}
