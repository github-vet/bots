package pointerescapes_test

import (
	"fmt"
	"testing"

	"github.com/kalexmills/github-vet/cmd/vet-bot/pointerescapes"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, pointerescapes.Analyzer, "devtest")
	fmt.Println(result[0].Result)
}
