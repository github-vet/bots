package writesinput_test

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/writesinput"
	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestWritesInput_BasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, writesinput.Analyzer, "writesinput_basic")

	assert.EqualValues(t, 1, len(results))
}
