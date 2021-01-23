package devtest

import (
	"fmt"

	"github.com/google/github"
)

func main() {
	var a A
	for _, z := range []int{1} { // want "suspicious use of range-loop variable" z:"variable z passed to unsafe call; reported as: WritesInput"
		var y int
		a.unsafeWrites(&z, &y)
	}
	for _, x := range []int{1, 2} { // want "suspicious use of range-loop variable" x:"variable x passed to unsafe call; reported as: CapturesAsync"
		unsafeAsync(&x)
	}
	for _, x := range []int{1, 2} {
		a.safe("hello", &x)
	}
	for _, x := range []int{1, 2, 3, 4} { // want "suspicious use of range-loop variable" x:"variable x passed to unsafe call; reported as: WritesInput"
		unsafeCallsAWrite(a, &x)
	}
	for _, y := range []int{1} { // want "suspicious use of range-loop variable" y:"variable y passed to unsafe call; reported as: WritesInput"
		unsafeCallsAWriteViaPointerLabyrinth(&y)
	}
	for _, w := range []int{1} {
		safe(&w)
	}
	for _, x := range []int{1} { // want "suspicious use of range-loop variable" x: "variable x passed to unsafe call; reported as: ExternalFunc"
		callExternal(&x)
	}
	for _, y := range []int{1} {
		callQualifiedIdentifier(&y)
	}
	var y UnsafeStruct
	for _, x := range []int{1, 2, 3} { // want "suspicious use of range-loop variable" x:"&x used inside a composite literal"
		y = UnsafeStruct{&x}
	}
	for _, y := range []int{1} { // want "suspicious use of range-loop variable" y:"&y used inside a composite literal"
		useUnsafeStruct(UnsafeStruct{&y})
	}
	var x *int
	for _, z := range []int{1} { // want "suspicious use of range-loop variable" z:"&z used on RHS of assign statement"
		x = &z
	}
	for _, z := range []int{2, 3, 4} { // want "suspicious use of range-loop variable" z:"&z used in a pointer comparison"
		if x == &z {
			fmt.Println("woohoo!")
		}
	}
	for _, z := range []int{1, 2, 3, 4} { // want "suspicious use of range-loop variable" z:"variable z passed to unsafe call; reported as: ComparesPtr"
		ptrCmp(&z, x)
	}
	fmt.Println(x, y) // for use
}

func useUnsafeStruct(x UnsafeStruct) {
	fmt.Println(x)
}

type UnsafeStruct struct {
	x *int
}

type A struct {
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

func safe(x *int) {
	safe1(x)
}
func safe1(x *int) {
	safe2(*x)
}

// callgraph should be cut off here, since safe2 does not pass a pointer.
func safe2(x int) {
	unsafeAsync(&x)
	unsafeCallsAWriteViaPointerLabyrinth(&x)
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
		*x = 3
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

func callExternal(x *int) {
	callThirdParty1(x)
}

func callThirdParty1(x *int) {
	callThirdParty2(x)
}

func callThirdParty2(x *int) {
	github.Execute(x)
}

func callQualifiedIdentifier(x *int) {
	callQualifiedIdentifier1(x)
}

func callQualifiedIdentifier1(x *int) {
	callQualifiedIdentifier2(x)
}

func callQualifiedIdentifier2(x *int) {
	fmt.Printf("%v", x) // fmt.Printf *is* accept-listed;
}

func ptrCmp(x *int, y *int) {
	ptrCmp1(x)
	safe(y)
}

func ptrCmp1(x *int) {
	ptrCmp2(x)
}

func ptrCmp2(x *int) {
	var y *int
	if x == y {
		fmt.Println("ack!")
	}
}
