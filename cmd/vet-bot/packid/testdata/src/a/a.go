package main

import (
	"fmt"
	"net/http"

	xy "github.com/google/go-github/github"
)

func main() {
	fmt.Println("hello")
	client := xy.NewClient(http.DefaultClient)
	fmt.Println(client)
}
