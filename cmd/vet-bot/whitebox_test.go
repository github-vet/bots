package main

import (
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescriptionTemplateCompiles(t *testing.T) {
	assert.NotPanics(t, func() {
		Description(VetResult{})
	})
}

func TestDescriptionTemplate(t *testing.T) {
	description := Description(VetResult{
		Repository: Repository{
			Owner: "owner",
			Repo:  "repo",
		},
		FilePath:     "file/path space/foo.go",
		RootCommitID: "rootcommitid",
		Quote:        "quote",
		Message:      "message",
		Start: token.Position{
			Filename: "file/path space/foo.go",
			Line:     123,
		},
		End: token.Position{
			Line: 125,
		},
		ExtraInfo: "extra",
	})

	// assert the important bits make it into the description properly
	assert.Contains(t, description, "```go\nquote\n```")
	assert.Contains(t, description, "```\nextra\n```")
	assert.Contains(t, description, "3 line(s) of Go")
	assert.Contains(t, description, "> message\n")
	assert.Contains(t, description, "[owner/repo](https://www.github.com/owner/repo)")
	assert.Contains(t, description, "[file/path space/foo.go](https://github.com/owner/repo/blob/rootcommitid/file/path%20space/foo.go#L123-L125)")
	assert.Contains(t, description, "[Click here to see the code in its original context.](https://github.com/owner/repo/blob/rootcommitid/file/path%20space/foo.go#L123-L125)")
}
