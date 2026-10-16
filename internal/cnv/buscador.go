package cnv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const buscadorURL = "https://www.cnv.gov.ar/SitioWeb/BuscadorGlobal/DataTableBuscadorGlobal"

// cuitPrefixes covers all Argentine corporate CUITs (30x, 33x, 34x).
// BuscadorGlobal requires >=3 chars; searching by CUIT prefix is exhaustive.
var cuitPrefixes = func() []string {
	var p []string
	for _, base := range []string{"30", "33", "34"} {
		for d := 0; d <= 9; d++ {
			p = append(p, fmt.Sprintf("%s%d", base, d))
		}
	}
	return p
}()

// Company is a CNV-registered entity with a public empresa page.
type Company struct {
	Name string
	CUIT string
	URL  string
}

type buscadorRequest struct {
	Draw               int             `json:"draw"`
	Columns            []buscadorCol   `json:"columns"`
	Order              []buscadorOrder `json:"order"`
	Start              int             `json:"start"`
	Length             int             `json:"length"`
	Search             buscadorSearch  `json:"search"`
	GReCaptchaResponse string          `json:"gRecaptchaResponse"`
}

type buscadorCol struct {
	Data       interface{}    `json:"data"`
	Name       string         `json:"name"`
	Searchable bool           `json:"searchable"`
	Orderable  bool           `json:"orderable"`
	Search     buscadorSearch `json:"search"`
}

type buscadorOrder struct {
	Column int    `json:"column"`
	Dir    string `json:"dir"`
}

type buscadorSearch struct {
	Value string `json:"value"`
	Regex string `json:"regex"`
}

type buscadorResponse struct {
	RecordsTotal int           `json:"recordsTotal"`
	Data         []buscadorRow `json:"data"`
}

type buscadorRow struct {
	Denominacion string  `json:"Denominacion"`
	IDFiscal     string  `json:"IDFiscal"`
	Url          *string `json:"Url"`
}

var baseColumns = []buscadorCol{
	{Data: "Denominacion", Searchable: true, Orderable: true, Search: buscadorSearch{Regex: "false"}},
	{Data: "IDFiscal", Searchable: true, Orderable: true, Search: buscadorSearch{Regex: "false"}},
	{Data: "Categoria", Searchable: true, Orderable: true, Search: buscadorSearch{Regex: "false"}},
	{Data: nil, Searchable: true, Orderable: true, Search: buscadorSearch{Regex: "false"}},
}

// FetchAllCompanies returns all CNV-registered companies with a public empresa page.
// It spawns numWorkers goroutines, each processing an equal slice of the 30 CUIT
// prefixes (30x/33x/34x) so coverage is exhaustive with no overlap.
func FetchAllCompanies(client *http.Client, numWorkers int) ([]Company, error) {
	prefixes := cuitPrefixes
	if numWorkers < 1 {
		numWorkers = 1
	}

	chunks := makeChunks(len(prefixes), numWorkers)

	type result struct {
		companies []Company
		err       error
	}
	ch := make(chan result, len(chunks))

	for _, c := range chunks {
		go func(slice []string) {
			batch, err := fetchPrefixSlice(client, slice)
			ch <- result{batch, err}
		}(prefixes[c[0]:c[1]])
	}

	seen := make(map[string]bool)
	var all []Company
	for range chunks {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		for _, company := range r.companies {
			if seen[company.CUIT] {
				continue
			}
			seen[company.CUIT] = true
			all = append(all, company)
		}
	}
	return all, nil
}

var cuitTermRE = regexp.MustCompile(`^\d{3,11}$`)

// SearchByCUIT returns the companies whose CUIT starts with term. term may be
// a full CUIT or a prefix of at least 3 digits; dashes and spaces are ignored
// because BuscadorGlobal stores IDFiscal as 11 bare digits. BuscadorGlobal
// also matches names, so results are filtered to CUIT prefix matches.
func SearchByCUIT(client *http.Client, term string) ([]Company, error) {
	term = strings.NewReplacer("-", "", " ", "").Replace(term)
	if !cuitTermRE.MatchString(term) {
		return nil, fmt.Errorf("invalid CUIT %q: want 3 to 11 digits", term)
	}

	batch, err := fetchOnePrefix(client, term)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var companies []Company
	for _, c := range batch {
		if seen[c.CUIT] || !strings.HasPrefix(c.CUIT, term) {
			continue
		}
		seen[c.CUIT] = true
		companies = append(companies, c)
	}
	return companies, nil
}

// fetchPrefixSlice fetches results for each prefix in the slice sequentially.
func fetchPrefixSlice(client *http.Client, prefixes []string) ([]Company, error) {
	var companies []Company
	for _, prefix := range prefixes {
		batch, err := fetchOnePrefix(client, prefix)
		if err != nil {
			return nil, err
		}
		companies = append(companies, batch...)
		time.Sleep(150 * time.Millisecond)
	}
	return companies, nil
}

// fetchOnePrefix calls the BuscadorGlobal for a single prefix term.
func fetchOnePrefix(client *http.Client, prefix string) ([]Company, error) {
	req := buscadorRequest{
		Draw:    1,
		Columns: baseColumns,
		Order:   []buscadorOrder{{Column: 0, Dir: "asc"}},
		Start:   0,
		Length:  9999,
		Search:  buscadorSearch{Value: prefix, Regex: ""},
	}
	body, _ := json.Marshal(req)

	resp, err := client.Post(buscadorURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("buscador POST %s: %w", prefix, err)
	}
	defer resp.Body.Close()

	var result buscadorResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("buscador decode %s: %w", prefix, err)
	}

	var companies []Company
	for _, row := range result.Data {
		if row.Url == nil || row.IDFiscal == "" {
			continue
		}
		companies = append(companies, Company{
			Name: html.UnescapeString(strings.TrimSpace(row.Denominacion)),
			CUIT: row.IDFiscal,
			URL:  *row.Url,
		})
	}
	return companies, nil
}

// makeChunks divides n items into at most numWorkers non-overlapping [start, end) pairs.
func makeChunks(n, numWorkers int) [][2]int {
	if numWorkers > n {
		numWorkers = n
	}
	if numWorkers < 1 {
		numWorkers = 1
	}
	size := (n + numWorkers - 1) / numWorkers
	var chunks [][2]int
	for start := 0; start < n; start += size {
		end := start + size
		if end > n {
			end = n
		}
		chunks = append(chunks, [2]int{start, end})
	}
	return chunks
}

// MakeChunks is the exported version used by main for company slices.
var MakeChunks = makeChunks

// GetCUITPrefixes returns the list of prefixes used to scan BuscadorGlobal.
func GetCUITPrefixes() []string { return cuitPrefixes }

// WorkersFor returns a sensible goroutine count for n items.
// Grows roughly as sqrt(n), capped at max.
func WorkersFor(n, max int) int {
	w := 1
	for w*w < n && w < max {
		w++
	}
	if w > max {
		w = max
	}
	if w < 1 {
		w = 1
	}
	return w
}
