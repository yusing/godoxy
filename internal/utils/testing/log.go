package expect

import "github.com/rs/zerolog"

func init() {
	if isTest {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}
