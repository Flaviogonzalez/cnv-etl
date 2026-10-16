package storage

import (
	"encoding/json"
	"os"
	"time"
)

// CompanyEntry is a single row in the company index.
type CompanyEntry struct {
	CUIT      string    `json:"cuit"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	FetchedAt time.Time `json:"fetched_at"`
}

// CompanyIndex is the full on-disk company cache.
type CompanyIndex struct {
	Version   int            `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	Companies []CompanyEntry `json:"companies"`
}

const indexVersion = 1

// SaveIndex writes the company list to path as JSON.
func SaveIndex(path string, entries []CompanyEntry) error {
	idx := CompanyIndex{
		Version:   indexVersion,
		CreatedAt: time.Now(),
		Companies: entries,
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadIndex reads the index file. Returns (entries, ok, error).
// ok is false if the file doesn't exist or is older than maxAge.
func LoadIndex(path string, maxAge time.Duration) ([]CompanyEntry, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	var idx CompanyIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, false, err
	}
	if idx.Version != indexVersion {
		return nil, false, nil
	}
	if maxAge > 0 && time.Since(idx.CreatedAt) > maxAge {
		return nil, false, nil
	}
	return idx.Companies, true, nil
}
