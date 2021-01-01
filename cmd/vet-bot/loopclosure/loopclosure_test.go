package loopclosure_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/loopclosure"

	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, loopclosure.Analyzer, "safe-usage")
}
