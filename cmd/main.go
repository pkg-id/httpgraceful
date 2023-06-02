package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/pkg-id/httpgraceful"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "Hello, World")
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", 8080),
		Handler: mux,
	}

	gs := httpgraceful.WrapServer(server)
	if err := gs.ListenAndServe(); err != nil {
		panic(err)
	}
}
