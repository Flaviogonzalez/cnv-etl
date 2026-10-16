package byma

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Flaviogonzalez/cnv-etl/internal/cnv"
)

const (
	bymaBase        = "https://open.bymadata.com.ar/vanoms-be-core/rest/api"
	allInstruments  = bymaBase + "/bymadata/free/instrumentproduct/instrument/all-instrument-select"
	fichatecnicaURL = bymaBase + "/bymadata/free/bnown/fichatecnica/especies/general"
)

// Emitter is a BYMA-listed Argentine company with its equity instrument code.
type Emitter struct {
	Symbol string
	Emisor string
	Lider  bool
}

// instrumentEntry is one item from all-instrument-select.
type instrumentEntry struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// fichatecnicaResp wraps the response from fichatecnica/especies/general.
type fichatecnicaResp struct {
	Data []struct {
		TipoEspecie string `json:"tipoEspecie"`
		Emisor      string `json:"emisor"`
		Lider       string `json:"lider"`
	} `json:"data"`
}

// suffixRE matches Argentine settlement-variant suffixes (B=CCL, C=24hs, D=inm, T=spot).
var suffixRE = regexp.MustCompile(`[BCDT]$`)

// FetchListedEmitters returns all BYMA-listed Argentine companies by:
//  1. Getting all EQUITYINSTRUMENT codes
//  2. Filtering to base codes (no settlement suffix)
//  3. Querying fichatecnica for each and keeping tipoEspecie=="Acciones"
func FetchListedEmitters(client *http.Client, workers int) ([]Emitter, error) {
	codes, err := fetchEquityCodes(client)
	if err != nil {
		return nil, err
	}

	// Keep only base codes (strip settlement suffixes to deduplicate).
	seen := make(map[string]bool)
	var baseCodes []string
	for _, c := range codes {
		base := c
		if suffixRE.MatchString(c) {
			base = c[:len(c)-1]
		}
		if !seen[base] {
			seen[base] = true
			baseCodes = append(baseCodes, base)
		}
	}

	if workers < 1 {
		workers = 1
	}
	return fetchFichatecnicaBatch(client, baseCodes, workers)
}

// MarkBYMACompanies sets the BYMA flag on companies in the CNV index.
// Returns the count of matches found.
func MarkBYMACompanies(emitters []Emitter, companies []cnv.Company) (map[string]bool, int) {
	byma := make(map[string]bool)
	for _, e := range emitters {
		norm := normalizeName(e.Emisor)
		for _, c := range companies {
			if normalizeName(c.Name) == norm || strings.Contains(normalizeName(c.Name), norm) || strings.Contains(norm, normalizeName(c.Name)) {
				byma[c.CUIT] = true
			}
		}
	}
	return byma, len(byma)
}

func fetchEquityCodes(client *http.Client) ([]string, error) {
	req, _ := http.NewRequest("GET", allInstruments, nil)
	req.Header.Set("Origin", "https://open.bymadata.com.ar")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("all-instruments GET: %w", err)
	}
	defer resp.Body.Close()

	var entries []instrumentEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("all-instruments decode: %w", err)
	}

	var codes []string
	for _, e := range entries {
		if e.Description == "EQUITYINSTRUMENT" {
			codes = append(codes, e.Code)
		}
	}
	return codes, nil
}

func fetchFichatecnicaBatch(client *http.Client, codes []string, workers int) ([]Emitter, error) {
	// Split codes into worker chunks.
	size := (len(codes) + workers - 1) / workers
	type result struct {
		emitters []Emitter
		err      error
	}
	ch := make(chan result, workers)

	for i := 0; i < len(codes); i += size {
		end := i + size
		if end > len(codes) {
			end = len(codes)
		}
		go func(slice []string) {
			var emitters []Emitter
			for _, code := range slice {
				e, err := fetchOneFichatecnica(client, code)
				if err != nil {
					// Non-fatal: skip missing symbols
					continue
				}
				if e != nil {
					emitters = append(emitters, *e)
				}
				time.Sleep(80 * time.Millisecond)
			}
			ch <- result{emitters, nil}
		}(codes[i:end])
	}

	var all []Emitter
	for i := 0; i < workers && i*size < len(codes); i++ {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		all = append(all, r.emitters...)
	}
	return all, nil
}

func fetchOneFichatecnica(client *http.Client, symbol string) (*Emitter, error) {
	body := fmt.Sprintf(`{"symbol":%q}`, symbol)
	req, _ := http.NewRequest("POST", fichatecnicaURL, strings.NewReader(body))
	req.Header.Set("Origin", "https://open.bymadata.com.ar")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ft fichatecnicaResp
	if err := json.NewDecoder(resp.Body).Decode(&ft); err != nil {
		return nil, err
	}

	for _, d := range ft.Data {
		if d.TipoEspecie == "Acciones" && d.Emisor != "" {
			return &Emitter{
				Symbol: symbol,
				Emisor: d.Emisor,
				Lider:  d.Lider == "Si",
			}, nil
		}
	}
	return nil, nil
}

// normalizeName strips common legal suffixes and normalizes whitespace for matching.
var normalizeRE = regexp.MustCompile(`\bS\.?A\.?U?\.?|\bS\.?R\.?L\.?|\bS\.?A\.?\b|[.,]`)

func normalizeName(s string) string {
	s = strings.ToUpper(s)
	s = normalizeRE.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}
