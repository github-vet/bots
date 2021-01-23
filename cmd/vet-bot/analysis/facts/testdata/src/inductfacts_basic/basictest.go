package inductfacts_basic

import (
	"fmt"

	"github.com/stretchr/testify/assert"
)

func useUnsafeStruct(x UnsafeStruct) {
	fmt.Println(x)
}

type UnsafeStruct struct {
	x *int
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

func (a *A) unsafeWrites(x, y *int) *int { // want x:"WritesInput" y:"WritesInput"
	return struct {
		x, y *int
	}{x, y}.x // why not?
}

func (a *A) safe(x string, y *int) *int {
	return nil
}

func unsafeCallsAWrite(a A, x *int) { // want x:"WritesInput"
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

func unsafeAsync(x *int) { // want x:"CapturesAsync"
	unsafeAsync1(x)
}
func unsafeAsync1(x *int) { // want x:"CapturesAsync"
	unsafeAsync2(x)
}
func unsafeAsync2(x *int) { // want x:"CapturesAsync"
	unsafeAsync3(x)
}
func unsafeAsync3(x *int) { // want x:"CapturesAsync"
	go func() {
		*x = 3
	}()
}

func unsafeAsyncDefer(x *int) { // want x:"CapturesAsync"
	unsafeAsyncDefer1(x)
}
func unsafeAsyncDefer1(x *int) { // want x:"CapturesAsync"
	unsafeAsyncDefer2(x)
}
func unsafeAsyncDefer2(x *int) { // want x:"CapturesAsync"
	defer func() {
		*x = 4
	}()
}

func unsafeCallsAWriteViaPointerLabyrinth(x *int) { // want x:"WritesInput"
	labyrinth1(3, "hello", x, 4.0)
}

func labyrinth1(x int, y string, z *int, w float32) { // want z:"WritesInput"
	forPtr := 3
	labyrinth2(y, z, &forPtr)
}

func labyrinth2(y string, z *int, w *int) { // want z:"WritesInput"
	labyrinth3(w, z, w)
}

func labyrinth3(x *int, z *int, y *int) { // want z:"WritesInput"
	labyrinth4(z, x, y)
}

func labyrinth4(z *int, x *int, y *int) { // want z:"WritesInput"
	writePtr(z)
} // okay so it's only a tiny labyrinth... :shrug:

func writePtr(x *int) { // want x:"WritesInput"
	var y *int
	y = x // 'write' is triggered here
	fmt.Println(y)
}

func callThirdParty(x *int) { // want x:"ExternalFunc"
	callThirdParty1(x)
}

func callThirdParty1(x *int) { // want x:"ExternalFunc"
	callThirdParty2(x)
}

func callThirdParty2(x *int) { // want x:"ExternalFunc"
	assert.Equal(nil, x)
}

func callThirdPartyAcceptListed(x *int) {
	callThirdPartyAcceptListed1(x)
}

func callThirdPartyAcceptListed1(x *int) {
	callThirdPartyAcceptListed2(x)
}

func callThirdPartyAcceptListed2(x *int) {
	fmt.Printf("%v", x) // fmt.Printf *is* accept-listed;
}

func usePtrCmp(x *int) { // want x:"ComparesPtr"
	usePtrCmp1(x)
}

func usePtrCmp1(x *int) { // want x:"ComparesPtr"
	usePtrCmp2(x)
}

func usePtrCmp2(x *int) { // want x:"ComparesPtr"
	var y *int
	if y == x {
		fmt.Println("ack!")
	}
}

func combinedBadness(x *int) { // want x:"WritesInput|ExternalFunc|ComparesPtr"
	usePtrCmp(x)
	callThirdParty(x)
	writePtr(x)
}

func combinedBadness1(x *int) { // want x:"WritesInput|ExternalFunc"
	callThirdParty(x)
	writePtr(x)
}

func combinedBadness2(x *int) { // want x:"ExternalFunc"
	callThirdParty(x)
	usePtrCmp(x)
}

func neverCalledPtrCmp(x *int) {
	// functions which are never called need not report anything
	y := 2
	if &y == x {
		fmt.Println("ack!")
	}
}

func nestedCall(x *int) { // want x:"NestedCallsite"
	nestedCall1(x)
}

func nestedCall1(x *int) { // want x:"NestedCallsite"
	nestedCall2(x)
}

func nestedCall2(x *int) { // want x:"NestedCallsite"
	func(x *int) { // yes;  it's a nested function call; no; nobody codes this way.
		fmt.Println(x)
	}(func(y *int) *int {
		return y
	}(x))
}
