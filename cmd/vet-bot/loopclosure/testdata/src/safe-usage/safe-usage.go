package main

import (
	"fmt"
	"sync"
)

func main() {

	wg := sync.WaitGroup{}
	x := []int{1, 2, 3, 4, 5}
	for _, v := range x { // want `range-loop variable v used in defer or goroutine at line 17`
		fmt.Println(v)
		if v == 4 {
			wg.Add(1)
			go func() {
				v += 10
				wg.Done()
			}()
			break
		}
	}
	wg.Wait()

}
