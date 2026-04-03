---
description: "Spec for rendering quality improvements — tracking remaining issues after dagre renderer implementation"
status: in-progress
status_description: "Dagre renderer working with real icons and nesting. Remaining: edge quality, layout tuning, D2 cleanup."
author: Michiel VH
created: 2026-04-03
---

# Plan: Rendering Quality

## Context

stackgraph has three renderers. The dagre renderer (custom SVG via dagre.js + goja) is the winner — it's the only one that produces self-contained SVGs with base64-embedded cloud provider icons, no system dependencies.

**Current state:**
- AWS Cloud boundary with official icon: working
- VPC/Subnet/SG container nesting with colored borders and icons: working
- Base64-embedded AWS Architecture Icons on resource nodes: working
- Compound graph layout via dagre.js: working

**Remaining issues:**

| Issue | Impact | Root Cause |
|-------|--------|------------|
| Too few edges visible | Diagram looks disconnected | Edge filter suppresses all edges to/from container nodes. Need to keep leaf-to-leaf flow edges. |
| Layout too horizontal for flat resources | S3, Lambda, Route53 spread in one row | No vertical hierarchy edges between standalone resources |
| Dagre can't route edges around containers | Edges cross through container borders | Known dagre limitation — no obstacle avoidance |
| D2 and Graphviz renderers still in codebase | Binary size bloat, maintenance burden | Need to remove D2 dependency and go-graphviz |

## Acceptance Criteria

- [ ] Multi-service example shows VPC with nested subnets, SGs, and resources visible as colored containers
- [ ] Edges between leaf resources (EC2 → RDS, ALB → EC2) render as visible arrows
- [ ] Standalone resources (S3, Lambda, Route53) positioned logically outside VPC
- [ ] D2 renderer and `oss.terrastruct.com/d2` dependency removed from go.mod
- [ ] go-graphviz renderer and `github.com/goccy/go-graphviz` dependency removed from go.mod
- [ ] `--renderer dagre` becomes the default (or only) SVG renderer
- [ ] Binary size reduced after removing D2 and go-graphviz dependencies
- [ ] rendering-spec.md updated with final status

## Implementation Tasks

### Edge improvements
- [ ] Keep edges between leaf nodes even if they cross container boundaries
- [ ] Only suppress edges that are direct parent→child containment (already shown via nesting)
- [ ] Add edge labels for meaningful connections (e.g., "port 5432" for DB connections)

### Cleanup
- [ ] Remove `pkg/output/svg.go` (D2 renderer)
- [ ] Remove `pkg/output/svg_graphviz.go` (Graphviz renderer)
- [ ] Remove D2 and go-graphviz from go.mod/go.sum
- [ ] Update `--renderer` flag: remove d2/graphviz options, make dagre the default
- [ ] Clean up unused D2/Graphviz example SVGs
- [ ] Run `go mod tidy` to remove unused dependencies

### Layout tuning
- [ ] Increase dagre `ranksep` for better vertical spacing
- [ ] Add edge weight support to dagre (heavier edges = closer nodes)
- [ ] Consider separate layout for VPC-internal vs external resources

### Test updates
- [ ] Update SVG tests to validate dagre output (base64 icons, self-contained)
- [ ] Remove D2/Graphviz-specific tests
