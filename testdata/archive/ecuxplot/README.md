# ARCHIVE: original ecuxplot KP packs (do not edit)

This directory is an archive. Nothing here is generated or updated by xdfkit.

- These are the original KP files as shipped in ecuxplot's `data/` (commit `a72b8d3`, 2026-10-06), kept so the exact originals remain available and as test input for xdfkit's KP reader.
- The `.csv` files are mapdump's output for them (ecuxplot `org.nyet.mappack`), frozen as the independent reference that xdfkit's KP reader is tested against. Never regenerate them with xdfkit.
- `images.tsv` names the ecu-corpus image for each archived pack. Tests use it (`../../../docs/corpus.md`).
- `8N0906018CB.bin` is the one image kept here: ecuxplot's non-OEM image (software 4019.XX, `24QVCGB1.HEX`, checksum errors), the only image the 8N0906018CB pack fits. The corpus image `8N0906018CB-0003` is different software, so lint tests use this bin.
- Add nothing new here.
