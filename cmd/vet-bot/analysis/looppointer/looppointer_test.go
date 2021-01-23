package looppointer_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/looppointer"
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
	analysistest.Run(t, testdata, looppointer.Analyzer, "devtest")
}
