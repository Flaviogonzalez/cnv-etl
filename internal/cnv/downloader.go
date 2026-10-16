package cnv

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const blobDownloadURL = "https://blob.cnv.gov.ar/BlobWebService.svc/DownloadBlob/%s"

// DownloadBlob downloads a file blob using its GUID and a fresh valet key.
// Returns the raw bytes of the PDF.
// Flow: GET valetKey → POST DownloadBlob with ValetKey form field.
func DownloadBlob(client *http.Client, fileGUID string) ([]byte, error) {
	valetKey, err := GetPublicValetKey(client, fileGUID)
	if err != nil {
		return nil, err
	}

	downloadURL := fmt.Sprintf(blobDownloadURL, fileGUID)

	formBody := url.Values{}
	formBody.Set("ValetKey", valetKey)

	req, err := http.NewRequest("POST", downloadURL, strings.NewReader(formBody.Encode()))
	if err != nil {
		return nil, fmt.Errorf("download new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CNV-Crawler/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download POST %s: %w", fileGUID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", fileGUID, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download read %s: %w", fileGUID, err)
	}
	return data, nil
}
