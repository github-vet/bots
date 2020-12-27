package looppointer_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/cmd/vet-bot/looppointer"
	"github.com/github-vet/bots/cmd/vet-bot/stats"
	"github.com/stretchr/testify/assert"
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

func TestStats(t *testing.T) {
	stats.Clear()
	testdata := analysistest.TestData()
	acceptlist.GlobalAcceptList = &acceptlist.AcceptList{
		Accept: map[string]map[string]struct{}{
			"fmt": {
				"Printf": {},
			},
		},
	}
	analysistest.Run(t, testdata, looppointer.Analyzer, "stattest")
	// validate only the stats looppointer is responsible for counting
	assert.EqualValues(t, stats.GetCount(stats.StatFuncDecl), 27)
	assert.EqualValues(t, stats.GetCount(stats.StatFuncCalls), 39)
	assert.EqualValues(t, stats.GetCount(stats.StatRangeLoops), 11)
	assert.EqualValues(t, stats.GetCount(stats.StatFuncCalls), 39)
	assert.EqualValues(t, stats.GetCount(stats.StatUnaryReferenceExpr), 22)
	assert.EqualValues(t, stats.GetCount(stats.StatLooppointerHits), 8)
	assert.EqualValues(t, stats.GetCount(stats.StatPtrFuncStartsGoroutine), 1)
	assert.EqualValues(t, stats.GetCount(stats.StatPtrFuncWritesPtr), 2)
	assert.EqualValues(t, stats.GetCount(stats.StatPtrDeclCallsThirdPartyCode), 1)
	assert.EqualValues(t, stats.GetCount(stats.StatLooppointerReportsWritePtr), 3)
	assert.EqualValues(t, stats.GetCount(stats.StatLooppointerReportsAsync), 1)
	assert.EqualValues(t, stats.GetCount(stats.StatLooppointerReportsThirdParty), 1)
	assert.EqualValues(t, stats.GetCount(stats.StatLooppointerReportsPointerReassigned), 1)
	assert.EqualValues(t, stats.GetCount(stats.StatLooppointerReportsCompositeLit), 2)
}
