package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescriptionTemplateCompiles(t *testing.T) {
	assert.NotPanics(t, func() {
		Description(VetResult{}, "")
	})
}
