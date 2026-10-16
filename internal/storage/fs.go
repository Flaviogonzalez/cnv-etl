package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PDFMeta is saved alongside each PDF as a .json sidecar.
type PDFMeta struct {
	CUIT               string    `json:"cuit"`
	CompanyName        string    `json:"company_name"`
	PresentationUUID   string    `json:"presentation_uuid"`
	FileGUID           string    `json:"file_guid"`
	IDEntidadPropiedad string    `json:"id_entidad_propiedad"`
	NombreArchivo      string    `json:"nombre_archivo"`
	FechaCierre        string    `json:"fecha_cierre,omitempty"`
	Description        string    `json:"description,omitempty"`
	DownloadedAt       time.Time `json:"downloaded_at"`
}

// SavePDF writes the PDF bytes and a JSON sidecar to baseDir/{cuit}/{fecha_cierre}_{guid}.pdf
func SavePDF(baseDir string, meta PDFMeta, data []byte) (string, error) {
	dir := filepath.Join(baseDir, sanitize(meta.CUIT))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}

	prefix := meta.FileGUID
	if meta.FechaCierre != "" {
		prefix = meta.FechaCierre + "_" + meta.FileGUID
	}

	pdfPath := filepath.Join(dir, prefix+".pdf")
	if err := os.WriteFile(pdfPath, data, 0644); err != nil {
		return "", fmt.Errorf("write pdf %s: %w", pdfPath, err)
	}

	metaPath := filepath.Join(dir, prefix+".json")
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		return "", fmt.Errorf("write meta %s: %w", metaPath, err)
	}

	return pdfPath, nil
}

// AlreadyDownloaded returns true if the PDF for this GUID already exists.
func AlreadyDownloaded(baseDir, cuit, fileGUID string) bool {
	dir := filepath.Join(baseDir, sanitize(cuit))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), fileGUID) && strings.HasSuffix(e.Name(), ".pdf") {
			return true
		}
	}
	return false
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(s)
}
