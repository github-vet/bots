package looppointer_test

import (
	"fmt"
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/looppointer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	acceptlist.GlobalAcceptList = &acceptlist.AcceptList{
		Accept: map[string]map[string]struct{}{
			"fmt": {
				"Printf": {},
			},
		},
	}
	result := analysistest.Run(t, testdata, looppointer.Analyzer, "devtest")
	fmt.Printf("%v\n", result[0].Result)
}
