package route

var (
	ImageNamePortMapTCP = map[string]int{
		"mssql":            1433,
		"mysql":            3306,
		"mariadb":          3306,
		"postgres":         5432,
		"pgvecto-rs":       5432,
		"pgvector":         5432,
		"rabbitmq":         5672,
		"redis":            6379,
		"memcached":        11211,
		"mongo":            27017,
		"minecraft-server": 25565,
	}

	ImageNamePortMapHTTP = map[string]int{
		"adguardhome":         3000,
		"bazarr":              6767,
		"calibre-web":         8083,
		"changedetection.io":  3000,
		"dockge":              5001,
		"gitea":               3000,
		"gogs":                3000,
		"grafana":             3000,
		"home-assistant":      8123,
		"homebridge":          8581,
		"httpd":               80,
		"immich":              3001,
		"jellyfin":            8096,
		"lidarr":              8686,
		"microbin":            8080,
		"nginx":               80,
		"nginx-proxy-manager": 81,
		"open-webui":          8080,
		"plex":                32400,
		"prometheus":          9090,
		"prowlarr":            9696,
		"radarr":              7878,
		"radarr-sma":          7878,
		"rsshub":              1200,
		"rss-bridge":          80,
		"sonarr":              8989,
		"sonarr-sma":          8989,
		"uptime-kuma":         3001,
		"whisparr":            6969,
	}
	ImageNamePortMapHTTPS = map[string]int{
		"portainer-be": 9443,
		"portainer-ce": 9443,
	}
	AliasPortMapHTTP  = map[string]int{}
	AliasPortMapHTTPS = map[string]int{
		"portainer": 9443,
		"crafty":    8080,
	}
)

func getSchemePortByImageName(imageName string) (scheme string, port int, ok bool) {
	if port, ok := ImageNamePortMapHTTP[imageName]; ok {
		return "http", port, true
	}
	if port, ok := ImageNamePortMapHTTPS[imageName]; ok {
		return "https", port, true
	}
	if port, ok := ImageNamePortMapTCP[imageName]; ok {
		return "tcp", port, true
	}
	return
}

func getSchemePortByAlias(alias string) (scheme string, port int, ok bool) {
	if port, ok := AliasPortMapHTTP[alias]; ok {
		return "http", port, true
	}
	if port, ok := AliasPortMapHTTPS[alias]; ok {
		return "https", port, true
	}
	return
}
