package common

import (
	"context"

	"github.com/sirrobot01/decypharr/pkg/debrid/types"
)

type NZBClient interface {
	SubmitNZB(ctx context.Context, dl *types.UsenetDownload) (*types.UsenetDownload, error)
	CheckNZBStatus(ctx context.Context, dl *types.UsenetDownload) (*types.UsenetDownload, error)
	GetNZBDownloadLink(usenetID string, file *types.File) (types.DownloadLink, error)
	DeleteNZB(id string) error
	IsNZBAvailable(hashes []string) map[string]bool
	GetNZBs() ([]*types.UsenetDownload, error)
	GetNZB(id string) (*types.UsenetDownload, error)
}

type NZBCapable interface {
	SupportsNZB() bool
	AsNZBClient() NZBClient
}
