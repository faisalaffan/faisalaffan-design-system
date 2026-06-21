package main

import (
	"fmt"
	"net/http"
)

func main() {
	fmt.Println("url-shortener running on :8080")
	http.ListenAndServe(":8080", nil)
}
