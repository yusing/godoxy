module github.com/yusing/godoxy/socketproxy

go 1.25.5

exclude github.com/yusing/goutils v0.4.2

replace github.com/yusing/goutils => ../goutils

require (
	github.com/gorilla/mux v1.8.1
	github.com/yusing/goutils v0.7.0
	golang.org/x/net v0.48.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.2.0 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)
