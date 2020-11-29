package main

func main() {

}

type A struct {
	x *int
}

func (a *A) twoRet() (int, int) {
	return 1, 2
}

func (a *A) foo3(x, y *int, z int) *int {
	y = x
	return x
}

func bar(x, y *int) A {
	unsafe(y)
	return A{x}
}

var z *int

func unsafe(x *int) *int {
	z = x
	return z
}

func safe(a *A) int {
	y := a.twoRet()
	return *y
}
