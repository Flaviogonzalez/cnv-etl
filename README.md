# cnv-etl

`cnv` busca empresas registradas en la Comisión Nacional de Valores (CNV) y descarga los PDF de sus estados contables. Opcionalmente, puede limitar la descarga a las empresas que cotizan en BYMA (Bolsas y Mercados Argentinos).

## Instalación

Requiere Go 1.26 o superior.

```bash
git clone https://github.com/Flaviogonzalez/cnv-etl.git
cd cnv-etl
go build -o cnv ./cmd/cnv
```

## Uso

```
cnv [flags]
```

Con `--buscar`, `cnv` procesa solo las empresas cuyo CUIT empieza con los dígitos indicados: un CUIT completo selecciona una sola empresa, y un prefijo más corto selecciona todas las que empiezan con él. Sin `--buscar`, procesa todas las empresas registradas en la CNV. Esa lista sale de un índice cacheado (`<out>/companies_index.json`), que se reconstruye cuando tiene más antigüedad que `--index-age` o cuando se pasa `--refresh`, porque recorrer todas las empresas lleva un rato.

| Flag | Default | Descripción |
|---|---|---|
| `--buscar <cuit>` | _(vacío)_ | CUIT o prefijo de CUIT a buscar (mínimo 3 dígitos, se aceptan guiones); vacío = todas las empresas |
| `--panel <p>` | _(vacío)_ | Solo empresas que cotizan en BYMA: `lider`, `general` o `all` |
| `--cierre <fecha>` | _(vacío)_ | Solo estados contables con esta FECHA CIERRE (`AAAA-MM-DD`) |
| `--out <dir>` | `pdfs` | Directorio de salida |
| `--dry-run` | false | Lista los archivos sin descargarlos |
| `--refresh` | false | Reconstruye el índice cacheado de empresas (se ignora con `--buscar`) |
| `--index-age <d>` | `168h` | Antigüedad máxima del índice cacheado |
| `--workers <n>` | 0 (auto) | Máximo de goroutines concurrentes |
| `--debug` | false | Log detallado de HTTP, que también se guarda en `--logs-dir` |
| `--logs-dir <dir>` | `logs` | Directorio para los logs de debug |

### Ejemplos

```bash
# Una empresa, por CUIT completo
cnv --buscar 30-50001091-2

# Todas las empresas cuyo CUIT empieza con 30500, sin descargar
cnv --buscar 30500 --dry-run

# Panel líder de BYMA, solo estados contables con cierre al 2025-12-31
cnv --panel lider --cierre 2025-12-31

# Reconstruir el índice de empresas y descargar todo con 10 workers
cnv --refresh --workers 10
```

## Salida

Los archivos que ya existen se saltean, así que volver a correrlo solo descarga los estados contables nuevos.

```
pdfs/
  companies_index.json            # índice cacheado de empresas de la CNV
  <cuit>/
    <fecha_cierre>_<guid>.pdf     # estado contable
    <fecha_cierre>_<guid>.json    # metadata: empresa, presentación, nombre del archivo, fechas
```
