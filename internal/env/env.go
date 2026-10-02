package env

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var envPrefixes = []string{"GODOXY_", "GOPROXY_", ""}

func SetPrefixes(prefixes ...string) {
	if len(prefixes) == 0 {
		envPrefixes = []string{""}
		return
	}
	envPrefixes = prefixes
}

// LookupEnv looks up an environment variable with the configured prefixes.
//
// It prefers the first non-empty value found, except HTTPS_ADDR where an empty
// value explicitly disables the shared HTTPS listener.
//
// Returns the value and a boolean indicating if an environment variable was found.
func LookupEnv(key string) (string, bool) {
	value, _, found := LookupEnvSource(key)
	return value, found
}

// LookupEnvSource also returns the selected variable name. Empty candidates do
// not shadow non-empty aliases, except HTTPS_ADDR. For other settings, source is
// empty if all candidates are empty.
func LookupEnvSource(key string) (value, source string, found bool) {
	for _, prefix := range envPrefixes {
		v, ok := os.LookupEnv(prefix + key)
		found = found || ok
		if ok && (v != "" || key == "HTTPS_ADDR") {
			return v, prefix + key, true
		}
	}
	return "", "", found
}

func GetEnv[T any](key string, defaultValue T, parser func(string) (T, error)) T {
	value, ok := LookupEnv(key)
	if !ok || value == "" {
		return defaultValue
	}
	parsed, err := parser(value)
	if err == nil {
		return parsed
	}
	log.Panicf("env %s: invalid %T value", key, parsed)
	return defaultValue
}

func GetEnvString(key string, defaultValue string) string {
	value, found := LookupEnv(key)
	if found && (value != "" || key == "HTTPS_ADDR") {
		return value
	}
	return defaultValue
}

func GetEnvBool(key string, defaultValue bool) bool {
	return GetEnv(key, defaultValue, strconv.ParseBool)
}

func GetEnvInt(key string, defaultValue int) int {
	return GetEnv(key, defaultValue, strconv.Atoi)
}

func GetAddrEnv(key, defaultValue, scheme string) (addr, host string, portInt int, fullURL string) {
	addr = GetEnvString(key, defaultValue)
	if addr == "" {
		return
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Panicf("env %s: invalid address", key)
	}
	fullURL = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, port))
	portInt, err = strconv.Atoi(port)
	if err != nil {
		log.Panicf("env %s: invalid port", key)
	}
	return
}

func GetEnvDuation(key string, defaultValue time.Duration) time.Duration {
	return GetEnv(key, defaultValue, time.ParseDuration)
}

func GetEnvCommaSep(key string, defaultValue string) []string {
	strs := strings.Split(GetEnvString(key, defaultValue), ",")
	for i, str := range strs {
		strs[i] = strings.TrimSpace(str)
	}
	return strs
}
