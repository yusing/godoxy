package maxmind

import "net"

type IPInfo struct {
	IP   net.IP
	Str  string
	City *City
}
