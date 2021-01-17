package main

import (
	"fmt"

	"github.com/haccer/available"
)

type A int

func foo(a *int) {
	if available.Domain("google.com") { // ignores external packages; marked as internal
		fmt.Println("google.com DNS entry expired!")
	}

	x := []int{1, 2, 3, 4}
	fmt.Println(len(x)) // ignores built-in -- not an external call

	fmt.Println(A(5)) // ignores casts to known types.
}
