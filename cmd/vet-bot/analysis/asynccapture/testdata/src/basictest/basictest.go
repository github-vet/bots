package main

func foo(x *int) { // want x:"funcAsyncCaptures"
	go func() {
		*x = 3
	}()
}

func bar(x *int, y *int) { // want x:"funcAsyncCaptures"
	defer func() {
		*x = 2
	}()
}

func asyncUse(x *int) { // want x:"funcAsyncCaptures"
	go bar2(x)
}

func asyncUse2(x *int, y *int) { // want y:"funcAsyncCaptures"
	go bar2(y)
}

func bar2(x *int) {
	*x = 3
}

func bar3(x *int, y *int) { // want x:"funcAsyncCaptures" y:"funcAsyncCaptures"
	go func() {
		*x = 3
		*y = 4
	}()
}
