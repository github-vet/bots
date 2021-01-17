package main

type I interface {
	A(x *int)
	B(y interface{})
	C(x ...*int)
	Foo(x int)
	Self() I
}

type A int

type B struct{}

func (a A) A(x *int)        {}
func (a A) B(y interface{}) {}
func (a A) C(x ...*int)     {}
func (a A) Foo(x int)       {}
func (a A) Self() I {
	return a
}

func (b *B) A(x *int) {
	b.B(x)
}
func (b *B) B(y interface{}) {}
func (b *B) C(x ...*int)     {}
func (b *B) Foo(x int)       {}
func (b *B) Self() I {
	return b
}

func foo(a A) {
	x := 2
	a.A(&x)
}

func bar(a *A) {
	y := 2
	a.B(&y)
}

func baz(a A, x *int) {
	a.C(x)
}

func NewA() A {
	return A(3)
}

func sel1() A {
	return NewA()
}

func selExp(x *int) {
	sel1().Self().A(x)
}
