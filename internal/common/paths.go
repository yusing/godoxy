package common

import (
	"path/filepath"
)

// file, folder structure

var (
	ConfigDir             = filepath.Join(RootDir, "config")
	ConfigFileName        = "config.yml"
	ConfigExampleFileName = "config.example.yml"
	ConfigPath            = filepath.Join(ConfigDir, ConfigFileName)

	MiddlewareComposeDir = filepath.Join(ConfigDir, "middlewares")
	ErrorPagesDir        = filepath.Join(RootDir, "error_pages")
	CertsDir             = filepath.Join(RootDir, "certs")

	DataDir        = filepath.Join(RootDir, "data")
	MetricsDataDir = filepath.Join(DataDir, "metrics")

	HomepageJSONConfigPath = filepath.Join(DataDir, "homepage.json")
	IconListCachePath      = filepath.Join(DataDir, "icon_list_cache.json")
	IconCachePath          = filepath.Join(DataDir, "icon_cache.json")
)

var RequiredDirectories = []string{
	ConfigDir,
	ErrorPagesDir,
	MiddlewareComposeDir,
}
