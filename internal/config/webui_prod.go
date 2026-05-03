//go:build production

package config

func webUIDevServerURL() (string, int, bool, error) {
	return "", 0, false, nil
}
