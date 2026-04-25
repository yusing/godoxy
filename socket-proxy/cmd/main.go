package main

import (
	"log"
	"net/http"

	socketproxy "github.com/yusing/godoxy/socketproxy/pkg"
)

func main() {
	if socketproxy.ListenAddr == "" {
		socketproxy.ListenAddr = "0.0.0.0:2375" // for the socket-proxy container
	}
	log.Printf("Docker socket listening on: %s", socketproxy.ListenAddr)
	http.ListenAndServe(socketproxy.ListenAddr, socketproxy.NewHandler())
}
