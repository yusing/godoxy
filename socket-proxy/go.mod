module github.com/yusing/godoxy/socketproxy

go 1.25.1

replace github.com/yusing/godoxy/internal/utils => ../internal/utils

require (
	github.com/gorilla/mux v1.8.1
	github.com/yusing/godoxy/internal/utils v0.0.0-20250922084459-f9affba9fc4e
	golang.org/x/net v0.44.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
