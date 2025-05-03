package qbittorrent

import (
	"context"
	"io"
)

func (c *Client) Version(ctx context.Context) (string, error) {
	resp, err := c.doRequest(ctx, "GET", "/api/v2/app/version", nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
