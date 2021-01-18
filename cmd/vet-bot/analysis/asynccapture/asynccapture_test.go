package asynccapture_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/asynccapture"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestInductFacts_BasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, asynccapture.Analyzer, "basictest")
}
