package nestedcallsite_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/nestedcallsite"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNestedCallsite_BasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nestedcallsite.Analyzer, "basic")
}
