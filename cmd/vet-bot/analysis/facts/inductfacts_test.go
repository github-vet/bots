package facts_test

import (
	"fmt"
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/facts"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestInductFacts_BasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, facts.InductFacts, "inductfacts_basic")

	fmt.Println(results[0].Result.(facts.InductResult))
}
