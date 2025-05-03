package maxmind

type City struct {
	Location struct {
		TimeZone string `maxminddb:"time_zone"`
	} `maxminddb:"location"`
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}
