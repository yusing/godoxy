package main

import (
	"log"
	"net/http"

	"math/rand/v2"
)

var printables = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
var random = make([]byte, 4096)

func init() {
	for i := range random {
		random[i] = printables[rand.IntN(len(printables))]
	}
}

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(random)
	})

	server := &http.Server{
		Addr:    ":80",
		Handler: handler,
	}

	log.Println("Bench server listening on :80")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
