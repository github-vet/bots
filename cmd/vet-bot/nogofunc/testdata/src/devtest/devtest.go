package main

func main() {

}

type A struct {
}
type B struct {
	a A
}

func (b *B) foo() {
	b.a.foo2(2)
}

func (a *A) foo() {
	var x, y int
	z := a
	z.foo2(x)

	a.foo3(&x, &y)
	a.foo4("1", &x)
	bar(&x)
	bar(&x)
}

func (a *A) foo2(x int) {
	go a.foo3(&x, &x)
}

func (a *A) foo3(x, y *int) *int {
	return nil
}

func (a *A) foo4(x string, y *int) *int {
	return nil
}

func bar(x *int) {

}

func bar2(x int) *int {
	return &x
}
