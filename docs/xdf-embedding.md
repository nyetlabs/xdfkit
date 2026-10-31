# Lossless data inside XDF

Specification for embedding the metadata file in an XDF comment block. Not implemented yet (`--embed`). The metadata file (`stamp-and-metadata.md`) is the primary lossless companion to an XDF. This block is optional and off by default. It carries the same payload as the metadata file (residue plus digests), XML encoded, not a second full model.

XML comments forbid `--` and a trailing `-`. Placement is one block inside `XDFFORMAT`, right after `XDFHEADER`, opened by a marker line:

```text
<!-- xdfkit-meta v1 xml
<xdfkit-meta schema="xdfkit-xdf-meta/1">
  <xdf>
    <digest canon="RFC8785" digest="sha256:..."/>
    <digest canon="jq -S ." digest="sha256:..."/>
  </xdf>
  <model>...</model>
  <object uniqueid="0x1" id="KFLDRQ2 (AR 27C02)" inverse="true" key="KFLDRQ2 (AR 27C02)">
    <category>2</category>
  </object>
  <stamp>...</stamp>
</xdfkit-meta>
-->
```

The mapping is the metadata file: object to element, scalar to attribute, list to repeated child elements. `objects` entries are keyed by XDF `uniqueid`.

Payload rules keep the outer comment well-formed, and the extracted payload parses with any stock XML parser:

- No comments, CDATA sections, processing instructions, or DOCTYPE inside the payload. Comments can't nest, and CDATA would let a raw `--` through.
- Text and attribute values: any `-` immediately following another `-` is written as `&#45;`, so `a--b` becomes `a-&#45;b`. All non-ASCII is written as `&#xNNNN;`, so the payload does not depend on the XDF's encoding (TunerPro files are often Windows-1252).
- Element and attribute names never contain `--`. The payload ends with `>` plus a newline, so the trailing-dash rule cannot trigger.

One block, not one per table: there is only one extraction point, and it is easy to strip. Reading uses the block like a metadata file when no `name.meta.json` exists, with the same digest-based merge rules. A metadata file, when present, is preferred.

TunerPro loads an XDF with a 119 KB block after `XDFHEADER` normally, but a TunerPro save drops it (confirmed), so the block survives only XDFs that TunerPro has not re-saved. The metadata file is the durable copy.
