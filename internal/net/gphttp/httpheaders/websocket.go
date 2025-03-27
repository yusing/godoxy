package httpheaders

import (
	"net/http"
)

const (
	HeaderXGoDoxyWebsocketAllowedDomains = "X-GoDoxy-Websocket-Allowed-Domains"
)

func WebsocketAllowedDomains(h http.Header) []string {
	return h[HeaderXGoDoxyWebsocketAllowedDomains]
}

func SetWebsocketAllowedDomains(h http.Header, domains []string) {
	h[HeaderXGoDoxyWebsocketAllowedDomains] = domains
}

func IsWebsocket(h http.Header) bool {
	return UpgradeType(h) == "websocket"
}
