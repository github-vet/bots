package main

func main() {
	var a A
	for _, x := range []int{1} {
		a.foo3(&x, &x)
	}
	for _, x := range []int{1} { // want `function call which takes a reference to x at line 9 may start a goroutine`
		bar(&x)
	}
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
	bar1(x)
}
func bar1(x *int) {
	bar2(x)
}
func bar2(x *int) {
	bar3(x)
}
func bar3(x *int) {
	go func() {

	}()
}
