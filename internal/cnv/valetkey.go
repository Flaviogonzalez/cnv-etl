package cnv

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const valetKeyURL = "https://aif2.cnv.gov.ar/api/ValetKeyProvider/GetPublicValetKey/%s?operation=DownloadBlob"

type valetKeyResponse struct {
	ValetKeyData string `json:"valetKeyData"`
}

// GetPublicValetKey fetches a short-lived download token for the given file GUID.
// The token must be used immediately in DownloadBlob.
func GetPublicValetKey(client *http.Client, fileGUID string) (string, error) {
	url := fmt.Sprintf(valetKeyURL, fileGUID)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CNV-Crawler/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("valetkey GET %s: %w", fileGUID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("valetkey %s: HTTP %d", fileGUID, resp.StatusCode)
	}

	var result valetKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("valetkey decode %s: %w", fileGUID, err)
	}
	if result.ValetKeyData == "" {
		return "", fmt.Errorf("valetkey empty for %s", fileGUID)
	}
	return result.ValetKeyData, nil
}
