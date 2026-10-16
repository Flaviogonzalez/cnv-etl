# cnv-etl

`cnv` searches companies registered with the Argentine securities regulator (Comisión Nacional de Valores, CNV) and downloads their financial statement PDFs (Estados Contables). It can optionally restrict the download to companies listed on BYMA (Bolsas y Mercados Argentinos).

## Installation

Requires Go 1.26+.

```bash
git clone https://github.com/Flaviogonzalez/cnv-etl.git
cd cnv-etl
go build -o cnv ./cmd/cnv
```

## Usage

```
cnv [flags]
```

With `--buscar`, `cnv` only processes the companies whose CUIT starts with the given digits, so a full CUIT selects a single company and a shorter prefix selects every company under it. Without `--buscar`, it processes every company registered with the CNV. That list comes from a cached index (`<out>/companies_index.json`), which is rebuilt when it is older than `--index-age` or when `--refresh` is set, because enumerating every company takes a while.

| Flag | Default | Description |
|---|---|---|
| `--buscar <cuit>` | _(empty)_ | CUIT or CUIT prefix to search (min 3 digits, dashes allowed); empty = all companies |
| `--panel <p>` | _(empty)_ | Only BYMA-listed companies: `lider`, `general` or `all` |
| `--cierre <date>` | _(empty)_ | Only statements with this FECHA CIERRE (`YYYY-MM-DD`) |
| `--out <dir>` | `pdfs` | Output directory |
| `--dry-run` | false | List files without downloading |
| `--refresh` | false | Rebuild the cached company index (ignored with `--buscar`) |
| `--index-age <d>` | `168h` | Max age for the cached company index |
| `--workers <n>` | 0 (auto) | Max concurrent goroutines |
| `--debug` | false | Verbose HTTP logging, also saved to `--logs-dir` |
| `--logs-dir <dir>` | `logs` | Directory for debug log files |

### Examples

```bash
# One company, by full CUIT
cnv --buscar 30-50001091-2

# Every company whose CUIT starts with 30500, without downloading
cnv --buscar 30500 --dry-run

# BYMA panel lider, only statements closed on 2025-12-31
cnv --panel lider --cierre 2025-12-31

# Rebuild the company index and download everything with 10 workers
cnv --refresh --workers 10
```

## Output

Files already present are skipped, so re-running only downloads new statements.

```
pdfs/
  companies_index.json            # cached CNV company index
  <cuit>/
    <fecha_cierre>_<guid>.pdf     # financial statement
    <fecha_cierre>_<guid>.json    # metadata: company, presentation, file name, dates
```
