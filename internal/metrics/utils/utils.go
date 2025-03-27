package metricsutils

import (
	"net/url"
	"strconv"
	"time"
)

func CalculateBeginEnd(n, limit, offset int) (int, int, bool) {
	if n == 0 || offset >= n {
		return 0, 0, false
	}
	if limit == 0 {
		limit = n
	}
	if offset+limit > n {
		limit = n - offset
	}
	return offset, offset + limit, true
}

func QueryInt(query url.Values, key string, defaultValue int) int {
	value, _ := strconv.Atoi(query.Get(key))
	if value == 0 {
		return defaultValue
	}
	return value
}

func QueryDuration(query url.Values, key string, defaultValue time.Duration) time.Duration {
	value, _ := time.ParseDuration(query.Get(key))
	if value == 0 {
		return defaultValue
	}
	return value
}
