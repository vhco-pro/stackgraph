---
description: "Spec for fixing and polishing both SVG renderers (D2 and Graphviz) to produce production-quality infrastructure diagrams"
status: in-progress
status_description: "Phase A+B complete — D2 duplicate-node fix and Graphviz cluster styling done. Phase C (real icons) and D (polish) remaining."
author: Michiel VH
created: 2026-04-03
---

# Rendering Quality Spec

## Context

stackgraph has two SVG renderers — D2 (default) and Graphviz (`--renderer graphviz`). Both currently produce subpar output due to specific bugs. This spec tracks the fixes needed to reach production quality.

The goal is Terravision-quality static diagrams: cloud service icons as nodes, nested VPC/subnet containers with colored borders, clean edge routing, professional typography.

## Current Issues

### D2 Renderer Issues

| # | Issue | Root Cause | Fix |
|---|-------|------------|-----|
| D1 | Duplicate nodes — resources appear both inside containers AND as flat nodes outside | D2 edge syntax `"aws_subnet.public" -> "aws_vpc.main"` creates top-level nodes because D2 interprets dots as hierarchy separators. The same nodes also exist inside their container. | Use D2's container-scoped edge references: edges between nested nodes must use their full container path (e.g., `"aws_vpc.main"."aws_subnet.public"`) |
| D2 | Broken icon placeholders | CDN URLs fail at render time — FIXED: switched to local embedded icons via temp files | Verify icons render correctly with local paths |
| D3 | Container label shows raw resource address | Label format not matching Terravision style | Use "Service — Name" format for labels |
| D4 | No visual distinction between public/private subnets | Both use same green color | Use different fill colors (green for public, blue for private) |

### Graphviz Renderer Issues

| # | Issue | Root Cause | Fix |
|---|-------|------------|-----|
| G1 | Cluster containers have no color or label | `sub.Set("bgcolor", ...)` and `sub.Set("label", ...)` not applying — go-graphviz's SubGraph `Set()` may need `SafeSet()` or attribute pre-declaration | Debug go-graphviz SubGraph attribute API |
| G2 | Icons not rendering — nodes are just text | `SetImage()` with temp SVG files — go-graphviz WASM may not support SVG images (Graphviz image support is format-dependent) | Try PNG icons instead of SVG, or use HTML labels with embedded images |
| G3 | Edges not rendering | Edge source/target node lookup uses original IDs (`aws_instance.web_a`) but gvNodes map uses sanitized IDs (`aws_instance_web_a`) | Fix ID mapping: store gvNodes by original ID, not sanitized |
| G4 | Security group and private subnet not visible | They are group-only nodes with no children and no leaf representation | Create invisible anchor nodes inside empty clusters so they render |
| G5 | No edge arrows | Default Graphviz arrows may be disabled | Explicitly set `arrowhead=normal` on edges |

## Acceptance Criteria

### D2 Renderer

- [ ] No duplicate nodes — each resource appears exactly once (inside its container if parented, or at root level if not)
- [ ] Edges between nested nodes route correctly through container boundaries
- [ ] Icons render inline (base64-embedded SVG) for all mapped resource types
- [ ] Container labels show "Service — Name" format
- [ ] VPC container has purple border (#8C4FFF)
- [ ] Subnet containers have green fill for public, blue fill for private
- [ ] Security group has red dashed border
- [ ] Resource nodes show icon + service name + resource name
- [ ] Count badge (x3) visible on collapsed count/for_each nodes
- [ ] Generic/unmapped resources render as clean labeled boxes

### Graphviz Renderer

- [ ] Cluster containers have colored backgrounds and labeled headers
- [ ] Resource nodes display icons (PNG format if SVG unsupported)
- [ ] All edges render with arrowheads
- [ ] Empty containers (private subnet with no children) render as visible boxes
- [ ] Edge routing works through cluster boundaries
- [ ] Node labels positioned below icons (labelloc=b)
- [ ] Font: Sans-Serif, #2D3436

### Both Renderers

- [ ] AWS simple-vpc test case produces clean, readable diagram
- [ ] Count-foreach test case shows collapsed nodes with badges
- [ ] Proxmox test case renders generic nodes without crashes
- [ ] SVG output is self-contained (no external file references)

## Implementation Tasks

### Phase A — Fix D2 duplicate-node bug (complete)

- [x] Rewrite `graphToD2()` to use D2's container-scoped node paths for edges
- [x] Build a node-path resolver that maps node IDs to their full D2 container path (`d2Paths` map)
- [x] Replace dot-separated IDs with underscore-safe IDs (`d2SafeID()`)
- [x] Edges between nested and root nodes now use correct D2 paths
- [x] Regenerate examples and verify no duplicates — confirmed 6 nodes, 7 edges

### Phase B — Fix Graphviz cluster attributes (complete)

- [x] Switched from programmatic API to DOT string generation + `cgraph.ParseBytes()` — go-graphviz's SubGraph API didn't apply attributes correctly
- [x] Clusters now have colored backgrounds, borders, labels, and font colors
- [x] Invisible anchor nodes added to empty clusters (e.g., private subnet with no children)
- [x] Edge IDs correctly sanitized and matched
- [x] Edges render with `#7B8894` color and arrowheads

### Phase C — Icon rendering

- [ ] Test go-graphviz with PNG icons (convert placeholder SVGs to PNGs)
- [ ] If Graphviz WASM doesn't support images, use HTML label approach: `<TABLE><TR><TD><IMG SRC="icon.png"/></TD></TR><TR><TD>label</TD></TR></TABLE>`
- [ ] For D2: verify local icon temp files are read and base64-embedded correctly
- [ ] Download 10 real AWS Architecture Icons (PNG) for visual testing

### Phase D — Polish both renderers

- [ ] Tune D2 theme and container styles for Terravision-like output
- [ ] Tune Graphviz node/edge spacing, fonts, colors
- [ ] Generate final side-by-side comparison examples
- [ ] Update rendering-analysis.md with results and final recommendation

## Test Plan

| Criterion | Test File |
|-----------|-----------|
| D2: no duplicate nodes in SVG output | `pkg/output/svg_test.go` |
| D2: icons embedded as base64 | `pkg/output/svg_test.go` |
| Graphviz: clusters have bgcolor | `pkg/output/svg_graphviz_test.go` |
| Graphviz: edges render | `pkg/output/svg_graphviz_test.go` |
| Both: empty graph renders placeholder | `pkg/output/svg_test.go` (existing) |
