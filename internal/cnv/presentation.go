package cnv

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	empresaURL    = "https://www.cnv.gov.ar/SitioWeb/Empresas/Empresa/%s"
	publicviewURL = "https://aif2.cnv.gov.ar/presentations/publicview/%s"
)

type Presentation struct {
	UUID        string
	Date        string
	Description string
	DocNumber   string
}

type FileAttachment struct {
	IDEntidadPropiedad string
	NombreArchivo      string
	Tamano             string
	GUID               string
	Hash               string
}

var (
	presentationLinkRE = regexp.MustCompile(`aif2\.cnv\.gov\.ar/presentations/publicview/([0-9a-f-]{36})`)
	// Each row: date, time, description, docnumber, link
	rowRE = regexp.MustCompile(`(?s)<tr>\s*<td>\s*([\d\w\s\.]+?)\s*</td>\s*<td>\s*[\d:]+\s*</td>\s*<td>([^<]+)</td>\s*<td>(\d+)</td>.*?publicview/([0-9a-f-]{36})`)

	// File attachment JSON fragments embedded in HTML
	attachmentRE = regexp.MustCompile(`"idEntidadPropiedad":"([^"]+)","nombreArchivo":"([^"]+)","tamano":"([^"]+)","guid":"([0-9a-f-]{36})","hash":"([^"]*)"`)
)

// FetchPresentations returns all "Estados Contables" presentations for a given CUIT.
// It scrapes the server-rendered HTML from cnv.gov.ar/SitioWeb/Empresas/Empresa/{CUIT}.
func FetchPresentations(client *http.Client, cuit string) ([]Presentation, error) {
	url := fmt.Sprintf(empresaURL, cuit)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("empresa GET %s: %w", cuit, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("empresa read %s: %w", cuit, err)
	}
	html := string(body)

	seen := make(map[string]bool)
	var presentations []Presentation
	matches := rowRE.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		desc := strings.TrimSpace(m[2])
		uuid := m[4]
		if !strings.Contains(strings.ToUpper(desc), "ESTADOS CONTABLES") || seen[uuid] {
			continue
		}
		seen[uuid] = true
		presentations = append(presentations, Presentation{
			Date:        strings.TrimSpace(m[1]),
			Description: desc,
			DocNumber:   m[3],
			UUID:        uuid,
		})
	}

	// Fallback: if the row regex misses some (layout variation), also collect all
	// publicview UUIDs that appear near "ESTADOS CONTABLES" text.
	if len(presentations) == 0 {
		presentations = fallbackExtract(html)
	}

	return presentations, nil
}

// fallbackExtract finds all presentation UUIDs when the row regex fails.
func fallbackExtract(html string) []Presentation {
	links := presentationLinkRE.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool)
	var out []Presentation
	for _, l := range links {
		uuid := l[1]
		if seen[uuid] {
			continue
		}
		seen[uuid] = true
		out = append(out, Presentation{UUID: uuid})
	}
	return out
}

// FetchAttachments fetches the AIF publicview page for a presentation and
// returns all file attachments embedded in the server-rendered HTML.
// No headless browser required — GUIDs are in the initial HTML.
func FetchAttachments(client *http.Client, uuid string) ([]FileAttachment, error) {
	url := fmt.Sprintf(publicviewURL, uuid)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CNV-Crawler/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publicview GET %s: %w", uuid, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("publicview read %s: %w", uuid, err)
	}

	matches := attachmentRE.FindAllStringSubmatch(string(body), -1)
	var files []FileAttachment
	seen := make(map[string]bool)
	for _, m := range matches {
		guid := m[4]
		if seen[guid] {
			continue
		}
		seen[guid] = true
		files = append(files, FileAttachment{
			IDEntidadPropiedad: m[1],
			NombreArchivo:      m[2],
			Tamano:             m[3],
			GUID:               guid,
			Hash:               m[5],
		})
	}
	return files, nil
}

// IsEstadoContable returns true if the attachment is a financial statement file.
func (f FileAttachment) IsEstadoContable() bool {
	return f.IDEntidadPropiedad == "E10009PABEstadoContable"
}

// ParseFechaCierre extracts the closing date from a presentation description.
// Example: "NORMA CONTABLE: NIIF - ... - FECHA CIERRE: 2025-12-31 - ..."
func ParseFechaCierre(desc string) (time.Time, error) {
	re := regexp.MustCompile(`FECHA CIERRE:\s*(\d{4}-\d{2}-\d{2})`)
	m := re.FindStringSubmatch(strings.ToUpper(desc))
	if m == nil {
		return time.Time{}, fmt.Errorf("no FECHA CIERRE in: %q", desc)
	}
	return time.Parse("2006-01-02", m[1])
}
