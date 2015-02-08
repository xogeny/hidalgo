package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Handler called")
	msg := os.Getenv("HELLO_MESSAGE")
	if msg == "" {
		msg = "I was built with Hidalgo (default_message)"
	}
	fmt.Fprintf(w, "%s\n", msg)
}

func main() {
	addr := ":8080"
	log.Printf("Running hidalgo/examples/hello at %v", addr)
	http.HandleFunc("/", handler)
	http.ListenAndServe(addr, nil)
}
