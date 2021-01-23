package packid_test

import (
	"fmt"
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/packid"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, packid.Analyzer, "a")
	fmt.Println(result[0].Result)
}
