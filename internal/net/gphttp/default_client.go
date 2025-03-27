package gphttp

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			ForceAttemptHTTP2: false,
			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 60 * time.Second, // this is different from DisableKeepAlives
			}).DialContext,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	Get  = httpClient.Get
	Post = httpClient.Post
	Head = httpClient.Head
)
