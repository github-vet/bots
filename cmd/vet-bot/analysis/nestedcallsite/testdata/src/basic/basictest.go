package main

import "fmt"

func main() {
	y := 3
	func(x *int) {
		foo(bar(x))
	}(foo(bar(&y)))
}

func test1(y *int, x *int) { // want y:"funcNestedCallsite"
	func(x *int) {
		foo(bar(x))
	}(foo(bar(y)))
}

func test2(y *int) { // want y:"funcNestedCallsite"
	foo(bar(y))
}

func test3(y *int, z *int) { // want y:"funcNestedCallsite" z:"funcNestedCallsite"
	func(x *int) {
		foo(bar(z))
	}(foo(bar(y)))
}

func test4(y *int) { // want y:"funcNestedCallsite"
	baz(bar(y)) // bar could pass y back to baz; our callgraph doesn't know
}

func test5(y *int) {
	fmt.Println(baz(y)) // we don't care that baz may pass its results back to Println.
}

func test6(y *int) { //  want y:"funcNestedCallsite"
	fmt.Println(bar(y)) // TODO: suppress this false-positive using the acceptlist.
}

func test7(y *int) { // want y:"funcNestedCallsite"
	// no; nobody codes like this; yes, we can handle it anyway.
	func(x *int) {
		fmt.Println(x)
	}(
		func(y *int) *int {
			return y
		}(y),
	)
}

func foo(x *int) *int {
	return bar(x)
}

func bar(x *int) *int {
	fmt.Println(x)
	return x
}

func baz(x *int) int {
	return *x
}
