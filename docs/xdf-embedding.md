# Lossless data inside XDF

Specification for embedding model data in an XDF comment block. The metadata file (`stamp-and-metadata.md`) is the primary lossless companion to an XDF. This block is optional (`--embed`), and by default carries the same payload as the metadata file (residue plus digests) rather than the full model. The rules below apply to whatever payload it carries.

- XML comments forbid `--` and a trailing `-`; nothing else is restricted. The payload is an XML serialization of the canonical model, mechanically mapped from the same schema as the JSON output (object to element, scalar to attribute, list to repeated child elements).
- Placement: one block inside `XDFFORMAT`, right after `XDFHEADER`, opened by a marker line with a schema version:

```text
<!-- xdfkit-model v1 xml
<xdfkit-model version="1">
  <shared>...</shared>
  <object id="KFMIOP" ...>
    <note>A2L CUBOID split into 4 XDF tables</note>
    ...
  </object>
</xdfkit-model>
-->
```

- Payload rules (they keep the outer comment well-formed, and the extracted payload parses with any stock XML parser):
  - No comments, CDATA sections, processing instructions, or DOCTYPE inside the payload. Comments can't nest, and CDATA would let a raw `--` through.
  - Anything that would be a comment is a `<note>` element instead. `<note>` is allowed as a child of any payload element and holds plain text: human-readable annotations such as lowering decisions, the source format, or a field's provenance.
  - `<note>` is informational only. The reader ignores it when building the model, and the writer regenerates notes on every export. Data never lives in a `<note>`.
  - Text and attribute values: any `-` immediately following another `-` is written as `&#45;`, so `a--b` becomes `a-&#45;b`. All non-ASCII is written as `&#xNNNN;`, so the payload doesn't depend on the XDF's encoding (TunerPro files are often Windows-1252).
  - Element and attribute names in the schema never contain `--`. The payload ends with `>` plus a newline, so the trailing-dash rule can't trigger.
- One block, not one per table: there is only one extraction point, it is easy to strip, and it carries shared objects naturally. Each object in it keys on its XDF `uniqueid`, like the metadata file.
- Reading: the block is used like a metadata file when no `name.meta.json` exists, with the same digest-based merge rules. A warning is printed if the block's object ids no longer match the XDF tables.
- Limitation (confirmed): TunerPro loads an XDF with a 119 KB block after `XDFHEADER` normally, but a TunerPro save drops it, so the block survives only XDFs that TunerPro hasn't re-saved. The metadata file is the durable copy.
