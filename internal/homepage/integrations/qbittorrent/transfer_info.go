package qbittorrent

import (
	"context"

	"github.com/yusing/go-proxy/internal/homepage/widgets"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

const endpointTransferInfo = "/api/v2/transfer/info"

type TransferInfo struct {
	ConnectionStatus string `json:"connection_status"`
	SessionDownloads uint64 `json:"dl_info_data"`
	SessionUploads   uint64 `json:"up_info_data"`
	DownloadSpeed    uint64 `json:"dl_info_speed"`
	UploadSpeed      uint64 `json:"up_info_speed"`
}

func (c *Client) Data(ctx context.Context) ([]widgets.NameValue, error) {
	info, err := jsonRequest[TransferInfo](ctx, c, endpointTransferInfo, nil)
	if err != nil {
		return nil, err
	}
	return []widgets.NameValue{
		{Name: "Status", Value: info.ConnectionStatus},
		{Name: "Download", Value: strutils.FormatByteSize(info.SessionDownloads)},
		{Name: "Upload", Value: strutils.FormatByteSize(info.SessionUploads)},
		{Name: "Download Speed", Value: strutils.FormatByteSize(info.DownloadSpeed) + "/s"},
		{Name: "Upload Speed", Value: strutils.FormatByteSize(info.UploadSpeed) + "/s"},
	}, nil
}
