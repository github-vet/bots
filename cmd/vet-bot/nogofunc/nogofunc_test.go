package nogofunc_test

import (
	"fmt"
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/nogofunc"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, nogofunc.Analyzer, "devtest")
	fmt.Printf("%v\n", result[0].Result)
}
