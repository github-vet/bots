package main

import "fmt"

func main() {
	y := 3
	func(x *int) {
		foo(bar(x))
	}(foo(bar(&y)))
}

func test1(y *int) { // want y:"funcNestedCallsite"
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

func foo(x *int) *int {
	return bar(x)
}

func bar(x *int) *int {
	fmt.Println(x)
	return x
}
