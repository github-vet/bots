package main

import "fmt"

func foo(x *int) { // want x:"funcComparesInput"
	var y = 3
	if x != &y {
		fmt.Println("ack!")
	}
}

func bar(x *int) { // want x:"funcComparesInput"
	var y = 3
	if &y == x {
		fmt.Println("ick!")
	}
}
