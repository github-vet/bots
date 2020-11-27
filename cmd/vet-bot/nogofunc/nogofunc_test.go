package nogofunc_test

import (
	"testing"

	"github.com/kalexmills/github-vet/cmd/vet-bot/nogofunc"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nogofunc.Analyzer, "devtest")
}
