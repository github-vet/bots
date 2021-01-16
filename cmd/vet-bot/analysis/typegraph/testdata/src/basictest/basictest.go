package main

import "fmt"

func main() {
	as := []A{{1, "1"}, {2, "2"}, {3, "3"}}
	for _, a := range as {
		foo(&a)
	}
}

type A struct {
	x int
	y string
}

type B struct {
	x int
}

func foo(a *A) {
	b(&a.x)
	d(&a.x, &a.y)
	a.interesting(2)
}

func b(x *int) {
	fmt.Println(x)
	uninteresting(*x)
}

func c(y *string) {
	fmt.Println(y)
	lessinteresting(*y)
}

func d(x *int, y *string) {
	fmt.Println(x, y)
}

func uninteresting(x int) {
	fmt.Println(x)
}

func lessinteresting(y string) {
	fmt.Println(y)
}

func (a *A) interesting(x int) {
	fmt.Println(x)
}

func (a A) reallyInterestingA(x interface{}) {
	fmt.Println(x)
}

func (b B) reallyInterestingB(x interface{}) {
	fmt.Println("naming collision!")
}
