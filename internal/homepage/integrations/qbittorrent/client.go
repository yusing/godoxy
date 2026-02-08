package qbittorrent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/bytedance/sonic"
	"github.com/yusing/godoxy/internal/homepage/widgets"
)

type Client struct {
	URL      string
	Username string
	Password string
}

func (c *Client) Initialize(ctx context.Context, url string, cfg map[string]any) error {
	c.URL = url
	c.Username = cfg["username"].(string)
	c.Password = cfg["password"].(string)

	_, err := c.Version(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) doRequest(ctx context.Context, method, endpoint string, query url.Values, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.URL+endpoint+query.Encode(), body)
	if err != nil {
		return nil, err
	}

	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	resp, err := widgets.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d %s", widgets.ErrHTTPStatus, resp.StatusCode, resp.Status)
	}

	return resp, nil
}

func jsonRequest[T any](ctx context.Context, client *Client, endpoint string, query url.Values) (result T, err error) {
	resp, err := client.doRequest(ctx, http.MethodGet, endpoint, query, nil)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	err = sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return result, err
	}

	return result, nil
}
