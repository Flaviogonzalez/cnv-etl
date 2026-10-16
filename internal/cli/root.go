package cli

import (
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cnv",
	Short: "Search CNV companies and download their financial statement PDFs",
	Long: `cnv searches companies registered with the Argentine securities regulator
(CNV) and downloads their financial statement PDFs (Estados Contables).

With --buscar it only processes the companies whose CUIT starts with the
given digits. Without it, it processes every CNV company, taken from a cached
index that is rebuilt when older than --index-age or when --refresh is set.`,
	Example: `  cnv --buscar 30-50001091-2              one company, by full CUIT
  cnv --buscar 30500 --dry-run            every company whose CUIT starts with 30500
  cnv --panel lider --cierre 2025-12-31   BYMA panel lider, one closing date
  cnv --refresh --workers 10              rebuild the index, then download everything`,
	Args:          cobra.NoArgs,
	RunE:          runScrape,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	scrapeOut        string
	scrapeBuscar     string
	scrapePanel      string
	scrapeCierre     string
	scrapeDryRun     bool
	scrapeRefresh    bool
	scrapeIndexAge   time.Duration
	scrapeMaxWorkers int
	scrapeDebug      bool
	scrapeLogsDir    string
)

func init() {
	f := rootCmd.Flags()
	f.StringVar(&scrapeOut, "out", "pdfs", "output directory for downloaded PDFs")
	f.StringVar(&scrapeBuscar, "buscar", "", "CUIT or CUIT prefix (min 3 digits, dashes allowed) to search; empty = all companies")
	f.StringVar(&scrapePanel, "panel", "", "only BYMA-listed companies: lider | general | all")
	f.StringVar(&scrapeCierre, "cierre", "", "only download statements with this FECHA CIERRE (YYYY-MM-DD)")
	f.BoolVar(&scrapeDryRun, "dry-run", false, "list files without downloading")
	f.BoolVar(&scrapeRefresh, "refresh", false, "rebuild the cached company index (ignored with --buscar)")
	f.DurationVar(&scrapeIndexAge, "index-age", 7*24*time.Hour, "max age for the cached company index")
	f.IntVar(&scrapeMaxWorkers, "workers", 0, "max goroutines (0 = auto-detect)")
	f.BoolVar(&scrapeDebug, "debug", false, "verbose HTTP logging; saves log to logs-dir")
	f.StringVar(&scrapeLogsDir, "logs-dir", "logs", "directory for debug log files")
}

func Execute() error {
	return rootCmd.Execute()
}
