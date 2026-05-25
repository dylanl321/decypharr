package types

import (
	"sync"
	"time"

	"github.com/sirrobot01/decypharr/pkg/arr"
)

type UsenetDownload struct {
	Id               string          `json:"id"`
	Hash             string          `json:"hash"`
	Name             string          `json:"name"`
	Filename         string          `json:"filename"`
	OriginalFilename string          `json:"original_filename"`
	Size             int64           `json:"size"`
	Bytes            int64           `json:"bytes"`
	Files            map[string]File `json:"files"`
	Status           TorrentStatus   `json:"status"`
	Added            time.Time       `json:"added"`
	Progress         float64         `json:"progress"`
	Speed            int64           `json:"speed"`
	Debrid           string          `json:"debrid"`
	Arr              *arr.Arr        `json:"arr"`
	NZBContent       []byte          `json:"-"`
	NZBLink          string          `json:"-"`
	DownloadUncached bool              `json:"-"`

	sync.Mutex
}

func (d *UsenetDownload) GetSize() int64 {
	if d.Size == 0 {
		return d.Bytes
	}
	return d.Size
}

func (d *UsenetDownload) GetFiles() []File {
	files := make([]File, 0, len(d.Files))
	for _, f := range d.Files {
		if !f.Deleted {
			files = append(files, f)
		}
	}
	return files
}

func (d *UsenetDownload) AsTorrent() *Torrent {
	t := &Torrent{
		Id:               d.Id,
		InfoHash:         d.Hash,
		Name:             d.Name,
		Filename:         d.Filename,
		OriginalFilename: d.OriginalFilename,
		Size:             d.Size,
		Bytes:            d.Bytes,
		Files:            d.Files,
		Status:           d.Status,
		Added:            d.Added,
		Progress:         d.Progress,
		Speed:            d.Speed,
		Debrid:           d.Debrid,
		Arr:              d.Arr,
		DownloadUncached: d.DownloadUncached,
	}
	if t.Files == nil {
		t.Files = make(map[string]File)
	}
	return t
}
