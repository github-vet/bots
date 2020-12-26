package acceptlist

import (
	"testing"

	"github.com/github-vet/bots/cmd/vet-bot/packid"
	"github.com/stretchr/testify/assert"
)

func TestAcceptListFromFile(t *testing.T) {
	list, err := AcceptListFromFile("testdata/acceptlist.yaml")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	assert.Contains(t, list.Accept, "fmt")
	assert.Contains(t, list.Accept["fmt"], "Println")
	assert.Contains(t, list.Accept["fmt"], "Printf")
	assert.Contains(t, list.Accept, "yaml")
	assert.Contains(t, list.Accept["yaml"], "Unmarshal")
}

func TestZeroValue(t *testing.T) { // not technically whitebox
	assert.NotPanics(t, func() {
		IgnoreCall(&packid.PackageResolver{}, nil, nil)
	})
}
