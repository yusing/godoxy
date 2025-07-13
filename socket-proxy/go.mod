module github.com/yusing/go-proxy/socketproxy

go 1.24.5

replace github.com/yusing/go-proxy/internal/utils => ../internal/utils

require (
	github.com/gorilla/mux v1.8.1
	github.com/yusing/go-proxy/internal/utils v0.0.0-20250628064654-5631b1540abc
	golang.org/x/net v0.41.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
)
