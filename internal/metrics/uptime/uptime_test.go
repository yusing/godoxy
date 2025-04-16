package uptime

import (
	"net/url"
	"testing"
)

func TestPoller(t *testing.T) {
	Poller.Test(t, url.Values{"limit": []string{"1"}})
}
