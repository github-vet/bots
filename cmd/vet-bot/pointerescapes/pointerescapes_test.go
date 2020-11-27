package pointerescapes_test

import (
	"testing"

	"github.com/kalexmills/github-vet/cmd/vet-bot/pointerescapes"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, pointerescapes.Analyzer, "devtest")
}
