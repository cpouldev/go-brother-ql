# Reference fixtures

These fixtures support the [development validation](../../docs/DEVELOPMENT.md).
Run their encoder comparisons from the project root:

```sh
go test . -run '^TestReference(Monochrome|ColorAndWideHead)$'
```

`ql800-62-checker.bin` was generated with Philipp Klaus's `brother_ql` at
commit `0dee94b5fc3721159bae556b8bd2d975f92b7b8b`, using its default conversion
options for QL-800, 62 mm continuous stock. The RGB source is 696×150, white
except pixels where `(x // 13 + y // 7) % 2 == 0`, which are black.

The standalone status-request command (`ESC i S`) is replaced at parsed command
boundaries with `ESC i ! 0`, enabling automatic status notifications as specified
on page 30 of Brother's QL-800 raster manual. The Go printing session owns status
requests. All other bytes are preserved. The fixture tests the encoder against
an independent implementation with this explicitly documented correction.

Source: [pinned brother_ql revision](https://github.com/pklaus/brother_ql/tree/0dee94b5fc3721159bae556b8bd2d975f92b7b8b).

License: GPL-3.0-or-later. Original author: Philipp Klaus and contributors.

Additional uncompressed fixtures use the same pinned revision and options:

| File | Source dimensions | Pattern |
| --- | --- | --- |
| `ql800-62red-checker.bin` | 696×150 | `[white, black, red][(x//13+y//7)%3]`, `red=True` |
| `ql1100-102-checker.bin` | 1164×301 | `[white, black][(x//13+y//7)%2]` |

Colors are RGB white `(255,255,255)`, black `(0,0,0)`, and red `(255,0,0)`.
Images use exact printable dimensions, avoiding resampling differences. Generate
with `BrotherQLRaster(model)` and `convert(raster, [image], label, red=...)`.
Normalize commands using upstream `reader.chunker(..., raise_exception=True)`:
replace each standalone `ESC i S` with `ESC i ! 0` for QL-800 and remove it for
QL-1100. Never replace matching bytes inside raster payloads. The reference
fixtures are checked in; running Go tests needs no Python dependencies.
