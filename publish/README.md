# Publishing map definitions

Each corpus definition (`defs/IMAGE.json`, a `corpus.tsv` row whose `def` is set) is one zip, `IMAGE-DATE.zip`, in `build/publish/stage/`. `index.php` is the directory page: it reads the zips in the directory, counts the maps in each definition, and gives each image its own columns for the ECU, engine, code, vehicle, year, gearbox, and market. Apache serves it instead of a listing. KP, CSV and XDF are generated from that JSON. A correction goes into the JSON, never into the zip.

## Layout

`DATE` is `YYYYMMDD`, the later of the last commit of the JSON and, for an ME7 image, `categories.json`. A commit whose message starts `chore(schema):` does not count. `make upload` is `rsync -a --delete` to `UPLOAD_DEST` (gitignored `local.mk`; `RSYNC_FLAGS=-n` is a dry run). A new date removes the previous zip. The first upload of this tree also removes the old loose files and `full/`.

`CORPUS` is `../ecu-corpus` when that checkout exists, else the `corpus/` submodule (`docs/corpus.md`). A shallow submodule may not reach the commit, so the date is wrong; use the full clone.

Members keep undated names, so an XDF and its metadata file share a stem:

- `IMAGE.json`, copied from the corpus
- `IMAGE.kp`, `IMAGE.csv`, `IMAGE.xdf`, `IMAGE.meta.json`
- when the image family matches `ME7`: `IMAGE-tuner.xdf`, `IMAGE-tuner.meta.json`, `IMAGE-tuner.csv`, from `xdfkit --tuner` and `categories.json`

CSV is the map list only, with no reference columns. `8N0906018CB` is not published: it has no corpus definition.

## Import

`make incoming` runs `import-incoming.sh` on every KP, XDF and model JSON in `testdata/local/incoming/` (or on the files given). The image is the longest `corpus.tsv` name the filename starts with, followed by `.` or `-`. `4D1907558.kp` matches neither `4D1907558-0002` nor `4D1907558-0004`.

An XDF needs `NAME.meta.json` beside it, otherwise that member of `build/publish/stage/IMAGE-DATE.zip`. A subset (`IMAGE-tuner.xdf`) is refused. Origin `located` is written only where the corpus JSON is missing or already located. `ORIGIN=`, `FORCE=1` and the reciprocal-conversion warning are unchanged (`docs/corpus.md`). Files from `incoming/` then move to `incoming/imported/`.

## me7-logger

`me7info publish` lists every parity image. Eligible means grade D or better, confidence not mostly low, and the corpus definition is missing or already `located`. `me7info publish --write` writes `defs/NAME.json` (`provenance.origin` `located`) and sets that row's `def` to `defs/NAME.json`. The corpus is `../ecu-corpus` when that checkout exists, else `corpus/` (`CORPUS=` overrides). The command does not commit, push, bump, or upload. A star after the tier in `me7info parity` means that image has no corpus definition. xdfkit does not read tiers.

`me7info onboard` is how a new image enters the parity set (me7-logger `testdata/parity/incoming/README.md`).

- `hand` or `damos`. `publish` skips them. Added maps and wrong addresses are edited into that JSON by hand.
- `import-incoming.sh` remains the importer for KP, XDF, and DAMOS.
