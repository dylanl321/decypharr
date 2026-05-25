package torbox

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/sirrobot01/decypharr/internal/config"
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
)

func (tb *Torbox) SupportsNZB() bool {
	return true
}

func (tb *Torbox) AsNZBClient() debrid.NZBClient {
	return tb
}

func (tb *Torbox) SubmitNZB(ctx context.Context, dl *types.UsenetDownload) (*types.UsenetDownload, error) {
	var data AddUsenetResponse
	var resp *http.Response
	var err error

	if len(dl.NZBContent) > 0 {
		resp, err = tb.doPostUsenetFile("/api/usenet/createusenetdownload", dl.NZBContent, dl.Filename, &data)
	} else if dl.NZBLink != "" {
		resp, err = tb.doPostForm("/api/usenet/createusenetdownload", map[string]string{
			"link": dl.NZBLink,
		}, &data)
	} else {
		return nil, fmt.Errorf("nzb content or link is required")
	}
	if err != nil {
		return nil, err
	}
	if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return nil, fmt.Errorf("torbox usenet API error: Status: %d", resp.StatusCode)
	}
	if data.Data == nil || data.Data.UsenetDownloadID == 0 {
		return nil, fmt.Errorf("error adding usenet download")
	}

	dl.Id = strconv.Itoa(data.Data.UsenetDownloadID)
	if data.Data.Hash != "" {
		dl.Hash = strings.ToUpper(data.Data.Hash)
	} else if dl.Hash == "" && len(dl.NZBContent) > 0 {
		sum := md5.Sum(dl.NZBContent)
		dl.Hash = strings.ToUpper(hex.EncodeToString(sum[:]))
	}
	dl.Debrid = tb.config.Name
	dl.Added = time.Now()
	return dl, nil
}

func (tb *Torbox) doPostUsenetFile(endpoint string, fileData []byte, filename string, result interface{}) (*http.Response, error) {
	if filename == "" {
		filename = "download.nzb"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(fileData); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, tb.Host+endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := tb.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if result != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.ContentLength != 0 {
		if err := decodeJSONBody(resp.Body, result); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func (tb *Torbox) CheckNZBStatus(ctx context.Context, dl *types.UsenetDownload) (*types.UsenetDownload, error) {
	updated, err := tb.GetNZB(dl.Id)
	if err != nil {
		return dl, err
	}
	downloadUncached := dl.DownloadUncached
	*dl = *updated
	dl.DownloadUncached = downloadUncached

	switch dl.Status {
	case types.TorrentStatusDownloaded:
		tb.logger.Info().Msgf("Usenet download: %s ready", dl.Name)
		return dl, nil
	case types.TorrentStatusDownloading:
		// NZB via debrid is always "uncached" — TorBox downloads from usenet providers
		return dl, nil
	default:
		return dl, fmt.Errorf("usenet download: %s has error", dl.Name)
	}
}

func (tb *Torbox) GetNZB(id string) (*types.UsenetDownload, error) {
	var res UsenetInfoResponse
	resp, err := tb.doGet("/api/usenet/mylist", map[string]string{"id": id}, &res)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("torbox usenet API error: Status: %d", resp.StatusCode)
	}
	if res.Data == nil {
		return nil, fmt.Errorf("usenet download not found")
	}
	return tb.mapUsenetInfo(res.Data), nil
}

func (tb *Torbox) GetNZBs() ([]*types.UsenetDownload, error) {
	offset := 0
	all := make([]*types.UsenetDownload, 0)
	for {
		batch, err := tb.getUsenetList(offset)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		offset += len(batch)
	}
	return all, nil
}

func (tb *Torbox) getUsenetList(offset int) ([]*types.UsenetDownload, error) {
	var res UsenetListResponse
	resp, err := tb.doGet("/api/usenet/mylist", map[string]string{
		"offset": strconv.Itoa(offset),
	}, &res)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("torbox usenet API error: Status: %d", resp.StatusCode)
	}
	if res.Data == nil || len(*res.Data) == 0 {
		return nil, nil
	}
	out := make([]*types.UsenetDownload, 0, len(*res.Data))
	for i := range *res.Data {
		out = append(out, tb.mapUsenetInfo(&(*res.Data)[i]))
	}
	return out, nil
}

func (tb *Torbox) mapUsenetInfo(data *torboxUsenetInfo) *types.UsenetDownload {
	dl := &types.UsenetDownload{
		Id:               strconv.Itoa(data.Id),
		Hash:             strings.ToUpper(data.Hash),
		Name:             data.Name,
		Filename:         data.Name,
		OriginalFilename: data.Name,
		Bytes:            data.Size,
		Progress:         data.Progress * 100,
		Status:           tb.getTorboxStatus(data.DownloadState, data.DownloadFinished),
		Speed:            data.DownloadSpeed,
		Debrid:           tb.config.Name,
		Added:            data.CreatedAt,
		Files:            make(map[string]types.File),
	}
	cfg := config.Get()
	for _, f := range data.Files {
		fileName := filepath.Base(f.Name)
		if err := cfg.IsFileAllowed(f.AbsolutePath, f.Size); err != nil {
			continue
		}
		file := types.File{
			TorrentId: dl.Id,
			Id:        strconv.Itoa(f.Id),
			Name:      fileName,
			Size:      f.Size,
			Path:      f.Name,
		}
		if data.DownloadFinished {
			file.Link = fmt.Sprintf("torbox-usenet://%s/%d", dl.Id, f.Id)
		}
		dl.Files[fileName] = file
	}
	var cleanPath string
	if len(data.Files) > 0 {
		cleanPath = path.Clean(data.Files[0].Name)
	} else {
		cleanPath = path.Clean(data.Name)
	}
	dl.OriginalFilename = strings.Split(cleanPath, "/")[0]
	return dl
}

func (tb *Torbox) GetNZBDownloadLink(usenetID string, file *types.File) (types.DownloadLink, error) {
	return tb.accountsManager.GetDownloadLink(usenetID, file, tb.fetchUsenetDownloadLink)
}

func (tb *Torbox) fetchUsenetDownloadLink(account *account.Account, id string, file *types.File) (types.DownloadLink, error) {
	query := url.Values{}
	query.Set("token", account.Token)
	query.Set("usenet_id", id)
	query.Set("file_id", file.Id)
	query.Set("redirect", "true")

	downloadURL := fmt.Sprintf("%s/api/usenet/requestdl?%s", tb.Host, query.Encode())
	now := time.Now()
	return types.DownloadLink{
		Filename:     file.Name,
		Size:         file.Size,
		Token:        tb.APIKey,
		Link:         file.Link,
		DownloadLink: downloadURL,
		Debrid:       tb.config.Name,
		Id:           file.Id,
		Generated:    now,
		ExpiresAt:    now.Add(tb.autoExpiresLinksAfter),
	}, nil
}

func (tb *Torbox) DeleteNZB(id string) error {
	payload := map[string]string{
		"usenet_id": id,
		"operation": "delete",
	}
	resp, err := tb.doPostForm("/api/usenet/controlusenetdownload", payload, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("torbox usenet API error: Status: %d", resp.StatusCode)
	}
	tb.logger.Info().Msgf("Usenet download %s deleted from Torbox", id)
	return nil
}

func (tb *Torbox) IsNZBAvailable(hashes []string) map[string]bool {
	result := make(map[string]bool)
	for i := 0; i < len(hashes); i += 100 {
		end := i + 100
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		var res UsenetCachedResponse
		params := map[string]string{
			"hash": strings.Join(batch, ","),
		}
		resp, err := tb.doGet("/api/usenet/checkcached", params, &res)
		if err != nil {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || !res.Success || res.Data == nil {
			continue
		}
		for h, c := range *res.Data {
			if c.Size > 0 {
				result[strings.ToUpper(h)] = true
			}
		}
	}
	return result
}

func decodeJSONBody(r io.Reader, result interface{}) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, result)
}
