# Minimal KP fixtures

Small complete packs for CI parse smoke tests. Not WinOLS production saves.

| File | Header | Source |
| --- | --- | --- |
| `v1-fewmaps.kp` | 116 | trim of `archive/ecuxplot/test8maps.kp` via `kp.Shrink` / Encode |
| `v2-fewmaps.kp` | 292 | trim of `archive/ecuxplot/8D0907551K.kp` via `kp.Shrink` / Encode |
| `245-fewmaps.kp` | 245 | trim of local AN pack (zip rebuild; Encode unsupported) |
| `503-fewmaps.kp` | 503 | trim of local DH pack |
| `597-fewmaps.kp` | 597 | trim of local BC pack |

Regenerate (245/503/597 need matching packs under `testdata/local/`):

```sh
go run ./internal/cmd/gen-kp-fixtures
```

`kp.Shrink` trims maps/folders then Encode (V1/V2) or rebuilds the `intern` zip (245/503/597). Full private saves stay in `testdata/local/` (gitignored). Do not add packs to `archive/ecuxplot/`.
