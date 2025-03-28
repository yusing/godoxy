package docker

var imageBlacklist = map[string]struct{}{
	// pure databases without UI
	"postgres":  {},
	"mysql":     {},
	"mariadb":   {},
	"redis":     {},
	"memcached": {},
	"mongo":     {},
	"rabbitmq":  {},
	"couchdb":   {},
	"neo4j":     {},
	"telegraf":  {},

	// search engines, usually used for internal services
	"elasticsearch": {},
	"meilisearch":   {},
	"kibana":        {},
	"solr":          {},
}

var imageBlacklistFullname = map[string]struct{}{
	// headless browsers
	"gcr.io/zenika-hub/alpine-chrome":      {},
	"eu.gcr.io/zenika-hub/alpine-chrome":   {},
	"us.gcr.io/zenika-hub/alpine-chrome":   {},
	"asia.gcr.io/zenika-hub/alpine-chrome": {},

	// image update watchers
	"watchtower": {},
	"getwud/wud": {},
}

var authorBlacklist = map[string]struct{}{
	// headless browsers
	"selenium":    {},
	"browserless": {},
	"zenika":      {},

	"zabbix": {},

	// docker
	"moby":   {},
	"docker": {},
}

func (image *ContainerImage) IsBlacklisted() bool {
	_, ok := imageBlacklist[image.Name]
	if ok {
		return true
	}
	_, ok = imageBlacklistFullname[image.Author+":"+image.Name]
	if ok {
		return true
	}
	_, ok = authorBlacklist[image.Author]
	return ok
}
