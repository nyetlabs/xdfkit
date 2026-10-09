# Publishing map definitions

The published packs (KP, CSV map list, XDF with its metadata file, and dated zips) are generated from the model JSON in ecu-corpus `defs/IMAGE.json`. That JSON is the source of truth: a correction goes into it, never into the generated files. `make help` lists the targets; outputs go to `build/publish/`.

## Bringing in a corrected definition

Edit the published KP in WinOLS, or the published XDF in TunerPro, and save it under a name that starts with the pack's stem from `testdata/archive/ecuxplot/images.tsv`, followed by `.` or `-`: `8D0907551K.kp`, `8D0907551K-kfvpdksd.xdf`. Then:

- Put the file in `testdata/local/incoming/` (gitignored). For an XDF, keep `NAME.meta.json` beside it if you have it. Without one, the script uses `build/publish/PACK.meta.json`, which matches the published XDF, so edits to that XDF are reconciled against it.
- Run `make -C publish incoming` (`import-incoming.sh`). For each file it runs `make import`, which rewrites the corpus JSON only when the definitions changed, stamp and provenance aside. The file, and a metadata file next to it, then move to `incoming/imported/`, so a later run can't import it over newer work.
- A corpus JSON whose stamp is edited (a person changed the JSON itself) is skipped. `FORCE=1` imports over it.
- A single file can be imported with `make -C publish import PACK=8D0907551K SRC=file.kp`, or `SRC=file.xdf META=file.meta.json`.

## After an import

- Commit the changed `defs/IMAGE.json` in the corpus checkout (`corpus/`, or `CORPUS=dir`) and push it. The corpus is append-only, so a correction is a normal commit; the zip date comes from that commit.
- Run `make -C publish` to regenerate the pack, `make -C publish zips` for the packs in `ZIP_BINS`, and `make -C publish upload` if `local.mk` defines it.
- Bump the `corpus` submodule here (`make corpus-bump`) and in the projects that read the corpus, such as me7-logger, whose parity oracles follow the same definitions.

## Packs

`PACKS` in the Makefile lists the packs. A pack without corpus JSON is converted from its archived KP in `testdata/archive/ecuxplot/`: 8N0906018CB, whose pack fits only the archive's non-OEM image. The archive is never edited, so that pack can't be corrected through `incoming` until an OEM image of its software is in the corpus.
