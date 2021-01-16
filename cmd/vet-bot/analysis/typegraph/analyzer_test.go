package typegraph_test

import (
	"fmt"
	"go/types"
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/typegraph"
	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestExternalCode(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, typegraph.Analyzer, "external")

	assert.EqualValues(t, 1, len(results))
	result := results[0].Result.(*typegraph.Result)
	fmt.Println(result)

	assert.NotEmpty(t, result.ExternalCalls)

	byId := make(map[string]*types.Func)
	for fun := range result.Declarations {
		byId[fun.Id()] = fun
	}

	cg := result.CallGraph
	assert.Len(t, cg.Calls(byId["external.foo"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["external.foo"])[0].Id())
}

func TestBasicTest(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, typegraph.Analyzer, "basictest")

	assert.EqualValues(t, 1, len(results))
	result := results[0].Result.(*typegraph.Result)

	byId := make(map[string]*types.Func)
	for fun := range result.Declarations {
		byId[fun.Id()] = fun
	}

	assert.Empty(t, result.ExternalCalls)

	cg := result.CallGraph
	assert.Len(t, cg.Calls(byId["basictest.foo"]), 3)
	assert.Contains(t, cg.Calls(byId["basictest.foo"]), byId["basictest.b"], byId["basictest.d"], byId["basictest.interesting"])

	assert.Len(t, cg.Calls(byId["basictest.b"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["basictest.b"])[0].Id())

	assert.Len(t, cg.Calls(byId["basictest.c"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["basictest.c"])[0].Id())

	assert.Len(t, cg.Calls(byId["basictest.d"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["basictest.d"])[0].Id())

	assert.Len(t, cg.Calls(byId["basictest.uninteresting"]), 0)
	assert.Len(t, cg.Calls(byId["basictest.lessinteresting"]), 0)

	assert.Len(t, cg.Calls(byId["basictest.main"]), 0)

	assert.Len(t, cg.Calls(byId["basictest.interesting"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["basictest.interesting"])[0].Id())

	assert.Len(t, cg.Calls(byId["basictest.reallyInterestingA"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["basictest.reallyInterestingA"])[0].Id())

	assert.Len(t, cg.Calls(byId["basictest.reallyInterestingB"]), 1)
	assert.Equal(t, "Println", cg.Calls(byId["basictest.reallyInterestingB"])[0].Id())
}
