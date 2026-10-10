# ARCHIVE: original ecuxplot KP packs (do not edit)

This directory is an archive. It is not where these definitions are maintained, and nothing here is generated or updated by xdfkit.

- The canonical definitions are the model JSON in ecu-corpus `defs/` (`../../../docs/corpus.md`; moved 2026-10-08, and ecuxplot's `data/` deleted), with KP, XDF and CSV generated from that JSON by xdfkit.
- These are the original KP files as published in ecuxplot's `data/` (commit `a72b8d3`, 2026-10-06), kept so the exact originals remain available and as test input for xdfkit's KP reader.
- The `.csv` files are mapdump's output for them (ecuxplot `org.nyet.mappack`), frozen as the independent reference that xdfkit's KP reader is tested against. Never regenerate them with xdfkit.
- `images.tsv` names the ecu-corpus image for each pack, and is the pack list `publish/` reads: it also names 4D1907558-0004, a pack with no archived KP (its corpus JSON is located by me7info); the tests use those OEM images. ecuxplot's patched 4Z7907551R image isn't kept (the corpus OEM image gives the same results).
- `8N0906018CB.bin` is the one image kept here: ecuxplot's non-OEM image (software 4019.XX, `24QVCGB1.HEX`, checksum errors), the only image the 8N0906018CB pack fits. The corpus OEM `8N0906018CB-0003` is different software. Replace it with an OEM image of the pack's software in the corpus once one is found.
- Add nothing new here except pack rows in `images.tsv`. New and changed definitions go to the corpus as JSON.
