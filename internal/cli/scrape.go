package cli

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Flaviogonzalez/cnv-etl/internal/byma"
	"github.com/Flaviogonzalez/cnv-etl/internal/cnv"
	"github.com/Flaviogonzalez/cnv-etl/internal/logger"
	"github.com/Flaviogonzalez/cnv-etl/internal/storage"
	"github.com/spf13/cobra"
)

func runScrape(cmd *cobra.Command, args []string) error {
	if scrapePanel != "" && scrapePanel != "lider" && scrapePanel != "general" && scrapePanel != "all" {
		return fmt.Errorf("--panel must be lider, general or all, got %q", scrapePanel)
	}
	if scrapeCierre != "" {
		if _, err := time.Parse("2006-01-02", scrapeCierre); err != nil {
			return fmt.Errorf("--cierre: %w", err)
		}
	}

	closeLog, err := logger.Setup(scrapeDebug, scrapeLogsDir)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer closeLog()

	client := newHTTPClient(scrapeDebug)

	if err := os.MkdirAll(scrapeOut, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", scrapeOut, err)
	}

	var companies []cnv.Company
	if scrapeBuscar != "" {
		companies, err = cnv.SearchByCUIT(client, scrapeBuscar)
		if err != nil {
			return fmt.Errorf("--buscar: %w", err)
		}
		log.Printf("found %d company(ies) matching CUIT %s", len(companies), scrapeBuscar)
	} else {
		indexPath := filepath.Join(scrapeOut, "companies_index.json")
		companies = loadOrFetchCompanies(client, indexPath, scrapeIndexAge, scrapeRefresh, scrapeMaxWorkers)
	}

	if scrapePanel != "" {
		companies = filterPanel(client, companies, scrapePanel, scrapeMaxWorkers)
	}
	if len(companies) == 0 {
		log.Println("no companies to process")
		return nil
	}

	log.Printf("processing %d companies", len(companies))
	workers := scrapeMaxWorkers
	if workers == 0 {
		workers = cnv.WorkersFor(len(companies), 15)
	}
	log.Printf("using %d worker(s)", workers)

	runConcurrent(client, companies, scrapeOut, scrapeDryRun, workers)
	return nil
}

func loadOrFetchCompanies(client *http.Client, indexPath string, maxAge time.Duration, forceRefresh bool, maxWorkers int) []cnv.Company {
	if !forceRefresh {
		entries, ok, err := storage.LoadIndex(indexPath, maxAge)
		if err != nil {
			log.Printf("WARN loading index: %v (will re-fetch)", err)
		} else if ok {
			log.Printf("loaded %d companies from index (%s)", len(entries), indexPath)
			return indexEntriesToCompanies(entries)
		}
	}

	log.Println("fetching company list from CNV BuscadorGlobal...")
	workers := maxWorkers
	if workers == 0 {
		workers = cnv.WorkersFor(len(cnv.GetCUITPrefixes()), 8)
	}
	log.Printf("buscador: %d goroutine(s) over %d prefixes", workers, len(cnv.GetCUITPrefixes()))

	companies, err := cnv.FetchAllCompanies(client, workers)
	if err != nil {
		log.Fatalf("fetch companies: %v", err)
	}
	log.Printf("found %d companies", len(companies))

	entries := companiesToIndexEntries(companies)
	if err := storage.SaveIndex(indexPath, entries); err != nil {
		log.Printf("WARN saving index: %v", err)
	} else {
		log.Printf("index saved → %s", indexPath)
	}
	return companies
}

// filterPanel keeps only companies whose BYMA equity belongs to the requested
// panel ("lider", "general", or "all" for any BYMA-listed equity).
func filterPanel(client *http.Client, all []cnv.Company, panel string, maxWorkers int) []cnv.Company {
	log.Println("fetching BYMA listed emitters...")
	workers := maxWorkers
	if workers == 0 {
		workers = 10
	}
	emitters, err := byma.FetchListedEmitters(client, workers)
	if err != nil {
		log.Fatalf("byma fetch: %v", err)
	}

	var inPanel []byma.Emitter
	for _, e := range emitters {
		if panel == "all" || e.Lider == (panel == "lider") {
			inPanel = append(inPanel, e)
		}
	}
	log.Printf("found %d BYMA equity emitters, %d in panel %s", len(emitters), len(inPanel), panel)

	panelMap, count := byma.MarkBYMACompanies(inPanel, all)
	log.Printf("matched %d CNV companies to panel", count)

	var matched []cnv.Company
	for _, c := range all {
		if panelMap[c.CUIT] {
			matched = append(matched, c)
		}
	}
	return matched
}

func runConcurrent(client *http.Client, companies []cnv.Company, outDir string, dryRun bool, numWorkers int) {
	chunks := cnv.MakeChunks(len(companies), numWorkers)

	type result struct{ errors []string }
	ch := make(chan result, len(chunks))
	var mu sync.Mutex

	for _, c := range chunks {
		go func(slice []cnv.Company) {
			workerClient := &http.Client{
				Timeout:   client.Timeout,
				Transport: client.Transport,
			}
			var errs []string
			for i, company := range slice {
				mu.Lock()
				log.Printf("[%s] %s (%d left in worker)", company.CUIT, company.Name, len(slice)-i-1)
				mu.Unlock()
				if err := processCUIT(workerClient, company.CUIT, company.Name, outDir, dryRun); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", company.CUIT, err))
				}
				time.Sleep(300 * time.Millisecond)
			}
			ch <- result{errs}
		}(companies[c[0]:c[1]])
	}

	var allErrors []string
	for range chunks {
		r := <-ch
		allErrors = append(allErrors, r.errors...)
	}
	if len(allErrors) > 0 {
		log.Printf("\n%d error(s):", len(allErrors))
		for _, e := range allErrors {
			log.Println(" ", e)
		}
		os.Exit(1)
	}
}

func processCUIT(client *http.Client, cuit, name, outDir string, dryRun bool) error {
	presentations, err := cnv.FetchPresentations(client, cuit)
	if err != nil {
		return fmt.Errorf("fetch presentations: %w", err)
	}
	if scrapeCierre != "" {
		var filtered []cnv.Presentation
		for _, p := range presentations {
			if t, err := cnv.ParseFechaCierre(p.Description); err == nil && t.Format("2006-01-02") == scrapeCierre {
				filtered = append(filtered, p)
			}
		}
		presentations = filtered
	}
	if len(presentations) == 0 {
		return nil
	}
	log.Printf("  %s: %d estado(s) contable(s)", cuit, len(presentations))

	workers := cnv.WorkersFor(len(presentations), 5)
	chunks := cnv.MakeChunks(len(presentations), workers)

	type presResult struct{ errors []string }
	pch := make(chan presResult, len(chunks))

	for _, c := range chunks {
		go func(slice []cnv.Presentation) {
			var errs []string
			for _, p := range slice {
				if err := processPresentation(client, cuit, name, outDir, dryRun, p); err != nil {
					errs = append(errs, err.Error())
				}
				time.Sleep(100 * time.Millisecond)
			}
			pch <- presResult{errs}
		}(presentations[c[0]:c[1]])
	}

	for range chunks {
		r := <-pch
		for _, e := range r.errors {
			log.Printf("  WARN %s: %v", cuit, e)
		}
	}
	return nil
}

func processPresentation(client *http.Client, cuit, name, outDir string, dryRun bool, p cnv.Presentation) error {
	fechaCierre := ""
	if t, err := cnv.ParseFechaCierre(p.Description); err == nil {
		fechaCierre = t.Format("2006-01-02")
	}

	attachments, err := cnv.FetchAttachments(client, p.UUID)
	if err != nil {
		return fmt.Errorf("attachments %s: %w", p.UUID, err)
	}

	for _, f := range attachments {
		if !f.IsEstadoContable() {
			continue
		}
		if storage.AlreadyDownloaded(outDir, cuit, f.GUID) {
			log.Printf("  SKIP %s", f.NombreArchivo)
			continue
		}
		if dryRun {
			log.Printf("  DRY-RUN %s (%s)", f.NombreArchivo, f.Tamano)
			continue
		}

		log.Printf("  ↓ %s (%s)", f.NombreArchivo, f.Tamano)
		data, err := cnv.DownloadBlob(client, f.GUID)
		if err != nil {
			return fmt.Errorf("download %s: %w", f.GUID, err)
		}

		meta := storage.PDFMeta{
			CUIT:               cuit,
			CompanyName:        name,
			PresentationUUID:   p.UUID,
			FileGUID:           f.GUID,
			IDEntidadPropiedad: f.IDEntidadPropiedad,
			NombreArchivo:      f.NombreArchivo,
			FechaCierre:        fechaCierre,
			Description:        p.Description,
			DownloadedAt:       time.Now(),
		}
		path, err := storage.SavePDF(outDir, meta, data)
		if err != nil {
			return fmt.Errorf("save %s: %w", f.GUID, err)
		}
		log.Printf("  ✓ %s", path)
	}
	return nil
}

func newHTTPClient(debug bool) *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &logger.Transport{
			Base:  http.DefaultTransport,
			Debug: debug,
		},
	}
}

func indexEntriesToCompanies(entries []storage.CompanyEntry) []cnv.Company {
	out := make([]cnv.Company, len(entries))
	for i, e := range entries {
		out[i] = cnv.Company{Name: e.Name, CUIT: e.CUIT, URL: e.URL}
	}
	return out
}

func companiesToIndexEntries(companies []cnv.Company) []storage.CompanyEntry {
	now := time.Now()
	out := make([]storage.CompanyEntry, len(companies))
	for i, c := range companies {
		out[i] = storage.CompanyEntry{CUIT: c.CUIT, Name: c.Name, URL: c.URL, FetchedAt: now}
	}
	return out
}
