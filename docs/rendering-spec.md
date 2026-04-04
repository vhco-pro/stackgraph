---
description: "Spec for stackgraph dagre renderer — tracking rendering quality and remaining improvements"
status: in-progress
status_description: "ELK left-to-right layout resolves edge-label overlap. Dark mode working. Container min-width enforced for labels."
author: Michiel VH
goal: "Production-quality static infrastructure diagrams with embedded icons, nested containers, and auto dark mode"
priority: high
created: 2026-04-03
---

# Plan: Rendering Quality

## Context

stackgraph uses a custom dagre.js renderer (via goja Go JS runtime) that produces self-contained SVGs with base64-embedded cloud provider icons. This is the primary renderer — D2 and go-graphviz are being removed.

## Current State (working)

- [x] AWS Cloud boundary with official AWS Cloud icon
- [x] VPC container with purple border (#8C4FFF) and VPC icon
- [x] Subnet containers with green fill (#F2F7EE) and subnet icon
- [x] Security Group containers with red dashed border (#E53935) and shield icon
- [x] Resource nodes with real AWS Architecture Icons (base64-embedded)
- [x] Compound graph layout via dagre.js (compound: true, setParent)
- [x] Container z-ordering — outermost drawn first (back), innermost last (front)
- [x] Inferred flow edges between resources (ALB→EC2, EC2→RDS, Lambda→S3, etc.)
- [x] Edge filtering — ancestor/descendant containment edges hidden, flow edges shown
- [x] Self-contained SVG — no external file references, portable
- [x] Three-tier architecture diagram rendering correctly

## Remaining Issues

### Edge routing quality

| Issue | Impact | Root Cause |
|-------|--------|------------|
| Lines cross through containers | Visual clutter on multi-service | Dagre polyline waypoints are straight segments, no obstacle avoidance |
| Multiple edges converge/diverge at same point | Messy line intersections | Dagre routes without separation between parallel edges |

**Approach:** Smooth the polyline waypoints into cubic bezier curves. This won't avoid obstacles but will make line crossings look cleaner.

### Dark mode support

SVG can auto-detect light/dark mode via CSS `@media (prefers-color-scheme: dark)`. Single SVG adapts automatically.

**Light mode (current):**
- Background: #FFFFFF
- Text: #2D3436
- Edge: #7B8894
- AWS Cloud border: #232F3E

**Dark mode (new):**
- Background: #1a1a2e
- Text: #E0E0E0
- Edge: #9CA3AF
- AWS Cloud border: #4A90D9
- Container fills: darken by 70% + increase saturation

### Cleanup

- Remove D2 renderer (`pkg/output/svg.go`) and `oss.terrastruct.com/d2` dependency
- Remove Graphviz renderer (`pkg/output/svg_graphviz.go`) and `github.com/goccy/go-graphviz` dependency
- Make dagre the default (and only) SVG renderer
- Run `go mod tidy` to purge unused deps

## Acceptance Criteria

### Edge quality
- [ ] Multi-service diagram has clean edge lines (no messy crossings)
- [ ] Edges use smooth curves instead of sharp polyline segments

### Dark mode
- [ ] SVG contains `@media (prefers-color-scheme: dark)` CSS block
- [ ] Background, text, edge, and container colors swap automatically
- [ ] Icons remain visible in dark mode (they have transparent backgrounds)
- [ ] Dark mode looks professional when viewed in dark-themed browser/IDE

### Cleanup
- [ ] D2 renderer removed
- [ ] go-graphviz renderer removed
- [ ] `go mod tidy` removes D2 and go-graphviz transitive dependencies
- [ ] Binary size reduced significantly
- [ ] `--renderer` flag removed, dagre is the only SVG renderer
- [ ] All SVG tests updated to test dagre output

## Implementation Tasks

- [ ] Add CSS `<style>` block with `@media (prefers-color-scheme: dark)` to SVG header
- [ ] Assign CSS classes to containers/nodes/edges for dark mode targeting
- [ ] Smooth edge paths — convert polyline waypoints to cubic bezier curves
- [ ] Remove `pkg/output/svg.go` and `pkg/output/svg_graphviz.go`
- [ ] Remove D2 and go-graphviz from go.mod
- [ ] Update CLI to remove `--renderer` flag
- [ ] Update `pkg/output/svg_test.go` for dagre-only output
- [ ] Regenerate all example SVGs
- [ ] Update README.md and architecture.md

## Test Plan

| Criterion | Test Type | Location |
|-----------|-----------|----------|
| SVG contains dark mode CSS media query | Unit | `pkg/output/svg_dagre_test.go` |
| SVG has no external file references | Unit | `pkg/output/svg_dagre_test.go` |
| SVG contains base64 icon data | Unit | `pkg/output/svg_dagre_test.go` |
| Container z-order correct (outermost first) | Unit | `pkg/output/svg_dagre_test.go` |
| Empty graph renders placeholder | Unit | `pkg/output/svg_dagre_test.go` |
