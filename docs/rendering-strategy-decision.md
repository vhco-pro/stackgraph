---
description: "Decision record for stackgraph's static rendering strategy — how to produce Terravision-quality SVGs with cloud provider icons"
status: proposed
author: Michiel VH
created: 2026-04-03
---

# Rendering Strategy Decision

## Problem

stackgraph needs to produce static SVG/PNG diagrams with cloud provider icons embedded. The current approach has two renderers, both with issues:

- **go-graphviz WASM**: Correct Terravision-style layout and colors, but the WASM sandbox **cannot render images**. Icons are silently stripped from SVG output. This is a fundamental limitation of the WASM Graphviz port — it has no filesystem access.
- **D2**: Can reference icons via temp file paths, but writes `href="/tmp/..."` instead of base64-embedding them. The SVG only works on the machine that generated it. Visual style is also different from Terravision (containers with labels vs icon-as-node).

Neither produces portable, production-quality output with icons.

## Options

### A: Shell out to system `dot` binary

**How:** Write DOT string to temp file, run `dot -Tsvg -o output.svg input.dot`, read result.

| Pro | Con |
|-----|-----|
| Terravision-identical output (they use the same approach) | Requires Graphviz system install (`apt install graphviz` / `brew install graphviz`) |
| Full image support — `image` attribute works perfectly | Not a single-binary distribution anymore |
| Battle-tested — this is how every Graphviz-based tool works | CI/CD and Docker need Graphviz in the image |
| Ortho splines, cluster nesting, HTML labels — all work | |

**Terravision uses this.** Every serious Graphviz-based diagram tool uses this. The WASM port is a convenience for simple graphs, not a production renderer.

### B: D2 with base64 icon embedding fix

**How:** Instead of passing temp file paths to D2, pre-read the icon files and base64-encode them as `data:image/png;base64,...` URIs in the D2 `icon:` attribute.

| Pro | Con |
|-----|-----|
| Single binary, no system deps | Different visual style from Terravision |
| Icons embedded inline (portable SVGs) | D2's container style doesn't match Terravision's "icon-as-node" approach |
| Good for general-purpose diagrams | Containers are labeled boxes, not cloud architecture diagrams |

### C: go-graphviz WASM + SVG post-processing

**How:** Render with go-graphviz WASM to get the layout, then parse the SVG XML and inject `<image>` elements with base64-encoded icon data at the node positions.

| Pro | Con |
|-----|-----|
| Single binary, no system deps | Complex SVG post-processing — fragile |
| Terravision-style layout | Need to parse SVG to find node positions and inject images |
| Icons embedded as base64 | Edge routing may not handle icon bounding boxes correctly (layout computed without images) |

### D: Hybrid — system `dot` with go-graphviz WASM fallback

**How:** Try system `dot` first. If not found, fall back to go-graphviz WASM (without icons, just colored containers with labels).

| Pro | Con |
|-----|-----|
| Best of both: full quality when Graphviz installed, still works without | Users without Graphviz get degraded output |
| Single binary distribution still works (degraded) | Need to clearly communicate the fallback |
| Same approach as many Go CLI tools (e.g., `terraform graph` output requires Graphviz to render) | |

## Recommendation

**Option D: Hybrid with system `dot` primary, WASM fallback.**

Rationale:
1. Terravision-quality output is the goal — that requires real Graphviz, not WASM
2. The `dot` binary is available on every major platform via package managers
3. Docker/CI environments can add `graphviz` to their base image trivially
4. The WASM fallback still produces useful (non-icon) diagrams for quick local use
5. The D2 renderer stays as a third option for users who prefer that style

The CLI would work like:
```bash
# Full quality (requires system Graphviz)
stackgraph generate --input state.json -f svg
# → Uses system dot binary, produces Terravision-quality SVG with icons

# If dot not found, warns and falls back to WASM
# "Warning: graphviz not found, icons will not render. Install with: apt install graphviz"

# Force D2 style
stackgraph generate --input state.json -f svg --renderer d2
```

For the Stackweaver platform integration, the Docker image includes Graphviz — so icons always work in production.

## Implementation

- [ ] Add `exec.LookPath("dot")` check in the Graphviz renderer
- [ ] If `dot` found: write DOT to temp file, exec `dot -Tsvg`, read output
- [ ] If `dot` not found: fall back to current go-graphviz WASM renderer with warning
- [ ] Keep D2 renderer as `--renderer d2` option
- [ ] Update README with install instructions for Graphviz
- [ ] Dockerfile already has the pattern for adding system deps
