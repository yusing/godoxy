module github.com/yusing/godoxy/socketproxy

go 1.25.1

replace github.com/yusing/godoxy/internal/utils => ../internal/utils

require (
	github.com/gorilla/mux v1.8.1
	github.com/yusing/goutils v0.2.1
	golang.org/x/net v0.44.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
