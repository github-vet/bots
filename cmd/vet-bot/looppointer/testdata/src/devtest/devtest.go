package main

import "fmt"

func main() {
	var a A
	for _, z := range []int{1} { // want `function call at line 9 may store a reference to z`
		var y int
		a.unsafeWrites(&z, &y)
	}
	for _, x := range []int{1, 2} { // want `function call which takes a reference to x at line 12 may start a goroutine`
		unsafeAsync(&x)
	}
	for _, x := range []int{1, 2} {
		a.safe("hello", &x)
	}
	for _, x := range []int{1, 2, 3, 4} { // want `function call at line 18 may store a reference to x`
		unsafeCallsAWrite(a, &x)
	}
	for _, y := range []int{1} { // want `function call at line 21 may store a reference to y`
		unsafeCallsAWriteViaPointerLabyrinth(&y)
	}
}

type A struct {
}
type B struct {
	a A
}

func (b *B) unsafeWritesNoArgs() {
	b.a.unsafeAsyncToWrite(2)
}

func (a *A) veryUnsafeNoArgs() {
	var x, y int
	z := a
	z.unsafeAsyncToWrite(x)

	a.unsafeWrites(&x, &y)
	a.safe("1", &x)
	unsafeAsync(&x)
	unsafeAsync(&x)
}

func (a *A) unsafeAsyncToWrite(x int) {
	go a.unsafeWrites(&x, &x)
}

func (a *A) unsafeWrites(x, y *int) *int {
	return struct {
		x, y *int
	}{x, y}.x // why not?
}

func (a *A) safe(x string, y *int) *int {
	return nil
}

func unsafeCallsAWrite(a A, x *int) {
	a.unsafeWrites(x, x)
}

func unsafeAsync(x *int) {
	unsafeAsync1(x)
}
func unsafeAsync1(x *int) {
	unsafeAsync2(x)
}
func unsafeAsync2(x *int) {
	unsafeAsync3(x)
}
func unsafeAsync3(x *int) {
	go func() {

	}()
}

func unsafeCallsAWriteViaPointerLabyrinth(x *int) {
	labyrinth1(3, "hello", x, 4.0)
}

func labyrinth1(x int, y string, z *int, w float32) { // z unsafe
	forPtr := 3
	labyrinth2(y, z, &forPtr)
}

func labyrinth2(y string, z *int, w *int) { // z unsafe
	labyrinth3(w, z, w)
}

func labyrinth3(x *int, z *int, y *int) {
	labyrinth4(z, x, y)
}

func labyrinth4(z *int, x *int, y *int) {
	writePtr(z)
} // okay so it's only a tiny labyrinth... :shrug:

func writePtr(x *int) {
	var y *int
	y = x // 'write' is triggered here
	fmt.Println(y)
}
