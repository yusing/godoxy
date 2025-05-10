package httpheaders

import (
	"net/http"
)

func IsWebsocket(h http.Header) bool {
	return UpgradeType(h) == "websocket"
}
