//go:build pprof

package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"runtime/debug"
)

func initProfiling() {
	runtime.GOMAXPROCS(2)
	debug.SetMemoryLimit(100 * 1024 * 1024)
	debug.SetMaxStack(15 * 1024 * 1024)
	go func() {
		log.Println(http.ListenAndServe(":7777", nil))
	}()
}
