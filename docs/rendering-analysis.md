# Rendering Engine Analysis: D2 vs go-graphviz

## Summary

**Keep D2. Do not add go-graphviz.** The broken icons in earlier output were caused by using remote CDN URLs — not a D2 limitation. Switching to bundled local SVG icons solves that completely. D2 produces better nested container output than Graphviz and embeds everything inline (portable SVGs). go-graphviz has a fundamental SVG limitation that would require custom post-processing.

## Comparison

| Aspect | D2 (current) | go-graphviz | Verdict |
|--------|-------------|-------------|---------|
| **License** | MPL-2.0 (file-level copyleft, Apache-2.0 compatible) | MIT wrapper + EPL-1.0 WASM (murkier) | D2 cleaner |
| **Nested containers** | Purpose-built, first-class | Subgraph clusters (functional but limited) | D2 wins |
| **Icon embedding in SVG** | Base64-inlined (self-contained, portable) | File path references only (SVG breaks if moved) | D2 wins |
| **Icon sources** | Local files (`icon: ./icons/foo.svg`) or URLs | Local files only (`image` attr) | Tie |
| **Non-cloud providers** | Generic labeled boxes, can add custom SVG icons | Generic labeled boxes, same | Tie |
| **Edge routing through containers** | Clean, handles nesting | Known issues with cluster edge routing | D2 wins |
| **Binary size** | ~28 MB (with all deps) | ~8 MB (WASM Graphviz) | go-graphviz smaller |
| **PNG output** | Requires headless browser (playwright) | Native rasterization | go-graphviz wins |
| **Rounded container borders** | Yes | No (Graphviz limitation) | D2 wins |
| **Maintenance** | One rendering pipeline | Two pipelines to maintain | D2 simpler |

## Licensing Detail

**D2 (MPL-2.0):** File-level copyleft — our Apache-2.0 code stays Apache-2.0. Only D2's own source files remain MPL-2.0. Since we import it as a dependency (not modifying D2's source), there is nothing to do beyond including the license notice. Per Mozilla's FAQ, MPL-2.0 is explicitly compatible with Apache-2.0.

**go-graphviz (MIT + EPL-1.0):** The Go wrapper is MIT, but it embeds Graphviz compiled to WASM. Graphviz's C source is EPL-1.0, which lacks the explicit Apache-2.0 compatibility clause that EPL-2.0 added. Usable but the licensing story requires more careful documentation.

**For an Apache-2.0 open-source project, D2's MPL-2.0 is actually cleaner than go-graphviz's EPL-1.0 WASM embedding.**

## The Broken Icons Issue

The broken image placeholders in our earlier SVG output were caused by using remote Terrastruct CDN URLs (`https://icons.terrastruct.com/aws/...`). D2's SVG renderer tries to fetch and base64-encode remote icons at compile time. If the fetch fails (offline, DNS issues, rate limiting), you get broken placeholders.

**Fix: bundle icons locally.** D2 supports `icon: ./icons/aws/ec2.svg` which reads the local file and base64-embeds it directly in the SVG. No CDN dependency, no fetch failures, fully offline.

This was fixed in D2 PR #2370 for remote URLs too, but local icons were never affected — they always work.

## Non-Cloud Provider Support

For providers without official icon sets (Proxmox, Kubernetes, Cloudflare, DigitalOcean, Hetzner, etc.):

- **With custom icons:** Create a simple SVG icon (even a text-based one) and reference it locally. D2 embeds it inline. Works for any provider.
- **Without icons:** Both D2 and Graphviz render plain labeled boxes. D2 looks slightly better because it applies the theme styling (rounded borders, shadows, colors).
- **Generic fallback:** Our YAML mapping system already handles this — unmapped resources render as generic nodes with the resource type as the label. The visual quality is the same regardless of rendering engine.

Bottom line: non-cloud providers look fine in D2 as generic boxes with labels. If someone contributes a Proxmox SVG icon set, adding it is trivial (drop SVGs in `icons/proxmox/`, add icon paths to `mappings/proxmox.yaml`).

## Why Not Both?

Using D2 for SVG and go-graphviz for PNG was considered but rejected:

1. **Maintenance cost** — two rendering pipelines means two icon-mapping tables, two layout strategies, two sets of styling parameters, two sets of bugs
2. **Marginal benefit** — D2 can produce PNG via headless browser if needed, or users can convert SVG→PNG with any tool (`rsvg-convert`, `inkscape --export-png`, browser)
3. **Binary size** — adding go-graphviz saves nothing (D2 is already included), only adds ~8 MB of WASM

## Action Items

1. **Download and embed cloud provider icon SVGs** in `stackgraph/icons/` via `go:embed`
2. **Switch `d2Icon()` from CDN URLs to local paths** (`./icons/aws/ec2.svg` instead of `https://icons.terrastruct.com/...`)
3. **Add a generic fallback icon** for unmapped resources
4. **Keep D2 as the sole rendering engine** — remove go-graphviz from the architecture docs
