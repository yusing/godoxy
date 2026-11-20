module github.com/yusing/godoxy/socketproxy

go 1.27.0

replace github.com/yusing/goutils => ../goutils

require (
	github.com/gorilla/mux v1.8.1
	github.com/yusing/goutils v0.9.4
	golang.org/x/net v0.63.0
)

require golang.org/x/text v0.44.0 // indirect
