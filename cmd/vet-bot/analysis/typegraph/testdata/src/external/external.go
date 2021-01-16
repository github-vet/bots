package main

import (
	"fmt"

	"github.com/haccer/available"
)

func foo(a *int) {
	if available.Domain("google.com") {
		fmt.Println("google.com DNS entry expired!")
	}
}
