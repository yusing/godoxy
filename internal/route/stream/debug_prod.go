//go:build !debug

package stream

import "github.com/rs/zerolog"

func logDebugf(stream zerolog.LogObjectMarshaler, format string, v ...any) {}
