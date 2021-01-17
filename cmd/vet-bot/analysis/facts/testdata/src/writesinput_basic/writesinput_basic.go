package main

func a(x *int) { // want x:"funcWritesInput"
	y := x
}

var X string

func foo(bar, x string) { // want bar:"funcWritesInput"
	X = bar
}

type A struct{}

var Y *A

func (a *A) foo() { // want a:"funcWritesInput"
	Y := a
}

type B struct {
	x *int
}

func baz(x *int) B { // want x:"funcWritesInput"
	return B{x}
}
