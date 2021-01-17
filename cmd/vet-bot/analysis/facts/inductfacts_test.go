package facts_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/facts"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestInductFacts_BasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, facts.InductionAnalyzer, "inductfacts_basic")
}
