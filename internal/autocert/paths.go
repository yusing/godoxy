package autocert

import (
	"os"
	"path/filepath"
)

const (
	certBasePath      = "certs/"
	CertFileDefault   = certBasePath + "cert.crt"
	KeyFileDefault    = certBasePath + "priv.key"
	runtimeBasePath   = "data/autocert"
	runtimeConfigName = "config.yml"
	helperBinaryName  = "autocert"
	helperBinaryEnv   = "GODOXY_AUTOCERT_BIN"
)

func RuntimeConfigPath() string {
	return filepath.Join(runtimeBasePath, runtimeConfigName)
}

func DefaultHelperBinary() string {
	if bin := os.Getenv(helperBinaryEnv); bin != "" {
		return bin
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), helperBinaryName)
	}
	return helperBinaryName
}

// AutocertConfigSnapshotPath keeps the old local name for tests and callers
// while RuntimeConfigPath is the helper-facing API.
func AutocertConfigSnapshotPath() string { return RuntimeConfigPath() }

// AutocertHelperBinary keeps the old local name for tests and callers while
// DefaultHelperBinary is the helper-facing API.
func AutocertHelperBinary() string { return DefaultHelperBinary() }
