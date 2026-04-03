---
description: "Spec for achieving Terravision-quality static SVG output from both D2 and Graphviz renderers"
status: in-progress
status_description: "Phase A+B complete (structural fixes). Phase C (Terravision-exact Graphviz params + real icons) in progress."
author: Michiel VH
goal: "Production-quality static infrastructure diagrams matching Terravision's visual output"
priority: high
created: 2026-04-03
---

# Plan: Rendering Quality — Terravision-Equivalent Output

stackgraph must produce production-quality static infrastructure diagrams. The visual benchmark is Terravision — proper cloud service icons as nodes, nested VPC/subnet containers with colored borders, clean edge routing, professional typography.

## Context

stackgraph has two SVG renderers sharing the same graph JSON:
- **D2** (default) — general-purpose diagram engine, good for containers and labels
- **Graphviz** (`--renderer graphviz`) — same engine Terravision uses, can reproduce their exact output

Both have been structurally fixed (Phase A/B) but need visual polish with correct parameters and real icon assets.

The exact Graphviz configuration used by Terravision has been extracted from their codebase and is documented in this spec.

## Scope

**In scope:**
- Graphviz renderer matching Terravision's exact visual parameters
- D2 renderer polished to best possible quality
- Real AWS Architecture Icon assets (PNG for Graphviz, SVG for D2)
- Side-by-side comparison for final renderer selection

**Out of scope:**
- Azure/GCP/Proxmox icon sets (future work after renderer is finalized)
- Interactive frontend rendering (@xyflow/react — separate plan)
- HTML output format

## Design

### Terravision's Exact Graphviz Configuration (extracted from source)

#### Graph Attributes

```
rankdir     = TB
splines     = ortho
overlap     = false
nodesep     = 3          (inches, horizontal)
ranksep     = 5          (inches, vertical)
pad         = 1.5        (inches)
fontname    = Sans-Serif
fontsize    = 30
fontcolor   = #2D3436
labelloc    = t
concentrate = false
center      = true
```

#### Default Node Attributes

```
shape      = box
style      = rounded
fixedsize  = true
width      = 1.4         (inches)
height     = 1.4         (inches)
labelloc   = b
imagepos   = c
imagescale = true
fontname   = Sans-Serif
fontsize   = 14
fontcolor  = #2D3436
```

When a node has an icon, these per-node overrides apply:

```
shape    = none           (icon becomes the node visual)
height   = 1.9            (+ 0.4 per newline in label)
image    = /abs/path/icon.png
labelloc = b              (label below icon)
```

#### Default Edge Attributes

```
color     = #7B8894
fontcolor = #2D3436
fontname  = Sans-Serif
fontsize  = 13
```

Labels use `xlabel` (external) with padding: `"  " + label + "  "`

#### AWS Container Attributes

| Container | style | border color | fill/bgcolor | margin | label |
|-----------|-------|-------------|-------------|--------|-------|
| AWS Cloud | solid, penwidth=2 | black | none | 100 | HTML table with AWS logo |
| VPC | solid | #8C4FFF | none | 50 | HTML table with VPC icon |
| Subnet (public) | filled | none (pencolor="") | #F2F7EE | 50 | HTML with subnet icon |
| Subnet (private) | filled | none (pencolor="") | #DEEBF7 | 50 | HTML with subnet icon |
| Security Group | solid | red | none | 50 | HTML with red font |
| Availability Zone | dashed | #3399FF | none | 100 | HTML blue font, size 30 |
| AutoScaling | dashed | pink | #DEEBF7 | 50 | HTML with ASG icon |

#### Rendering Pipeline (two-pass)

Terravision uses a two-pass process:
1. `dot` engine generates DOT with node positions
2. `gvpr` script adjusts label positions
3. `neato -n2` re-renders with preserved positions + ortho edge routing

**go-graphviz simplification:** We skip the gvpr step (label positioning) and use `dot` engine directly with `splines=ortho`. The two-pass neato approach is for cosmetic label placement — we can add it later if needed.

### Icon Strategy

Icons are referenced as **absolute file paths** to temp files written from `go:embed`.

| Provider | Format | Source |
|----------|--------|--------|
| AWS | PNG (64x64) | Official AWS Architecture Icons |
| Generic | PNG (64x64) | Simple labeled squares |

For MVP, use the placeholder SVG icons (already created) — go-graphviz's WASM engine may or may not render SVG images. If SVG fails, convert to PNG at build time.

## Acceptance Criteria

- [ ] Graphviz renderer uses Terravision's exact graph/node/edge attributes from the table above
- [ ] VPC containers have purple border (#8C4FFF), no fill, margin 50, HTML label with icon
- [ ] Public subnet containers are filled #F2F7EE with no border
- [ ] Private subnet containers are filled #DEEBF7 with no border
- [ ] Security group containers have red border
- [ ] Resource nodes with icons render as `shape=none` with icon image and label below
- [ ] Resource nodes without icons render as `shape=box, style=rounded`
- [ ] Edges use #7B8894 color with arrowheads
- [ ] Edge labels use `xlabel` attribute
- [ ] D2 renderer produces clean output with no duplicate nodes (Phase A — done)
- [ ] Generated SVGs are self-contained (no external file references)
- [ ] Side-by-side comparison examples generated for both renderers

## Implementation Phases

### Phase A: D2 Structural Fix (complete)

- [x] Rewrite edge path resolution to use container-scoped D2 paths
- [x] Replace dot-separated IDs with underscore-safe IDs
- [x] Verify 6 nodes, 7 edges in test output

### Phase B: Graphviz Structural Fix (complete)

- [x] Switch to DOT string generation + `cgraph.ParseBytes()`
- [x] Clusters render with colored backgrounds and labels
- [x] Edge ID sanitization matches node creation

### Phase C: Graphviz Terravision Parameters (current)

- [ ] Apply exact graph attributes: `splines=ortho`, `nodesep=3`, `ranksep=5`, `pad=1.5`, `overlap=false`
- [ ] Apply exact node defaults: `fixedsize=true`, `width=1.4`, `height=1.4`, `labelloc=b`, `imagescale=true`
- [ ] Apply per-node icon overrides: `shape=none`, `height=1.9`, `image=<path>`
- [ ] Apply exact edge defaults: `color=#7B8894`, `fontcolor=#2D3436`, `fontsize=13`
- [ ] Fix container attributes: VPC margin=50, pencolor=#8C4FFF; Subnet filled with no border; SG red border
- [ ] Use HTML table labels for containers (icon + name)
- [ ] Edge labels use `xlabel` instead of `label`
- [ ] Suppress edges between parent and child nodes that are already visually nested (reduces clutter)

### Phase D: Icon Assets

- [ ] Download 10 core AWS Architecture Icons (PNG 64x64): EC2, VPC, Subnet, SG, S3, RDS, Lambda, ALB, Route53, IAM
- [ ] Embed as PNG in `icons/aws/` via `go:embed`
- [ ] Test icon rendering in both Graphviz and D2
- [ ] If Graphviz WASM doesn't render images, use HTML label approach with base64-encoded PNGs

### Phase E: Final Comparison

- [ ] Generate aws-vpc example with both renderers
- [ ] Generate count-foreach example with both renderers
- [ ] Generate proxmox generic-fallback example with both renderers
- [ ] Document visual comparison in rendering-analysis.md
- [ ] Make recommendation on default renderer

## Test Plan

| Criterion | Test Type | Test Location |
|-----------|-----------|---------------|
| Graphviz uses exact Terravision attrs | Unit | `pkg/output/svg_graphviz_test.go` |
| D2 no duplicate nodes | Unit | `pkg/output/svg_test.go` (existing) |
| Empty graph renders placeholder | Unit | `pkg/output/svg_test.go` (existing) |
| SVG is self-contained (no external refs) | Unit | `pkg/output/svg_test.go` |
| Icons render in Graphviz SVG | Integration | Manual visual inspection |
| Icons render in D2 SVG | Integration | Manual visual inspection |
