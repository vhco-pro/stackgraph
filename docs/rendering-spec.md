---
description: "Spec for stackgraph rendering quality — tracking layout engines, edge routing, and visual improvements"
status: in-progress
status_description: "ELK left-to-right layout resolves edge-label overlap. Dark mode working. Container min-width enforced for labels."
author: Michiel VH
goal: "Production-quality static infrastructure diagrams with embedded icons, nested containers, and auto dark mode"
priority: high
created: 2026-04-03
---

# Plan: Rendering Quality

## Context

stackgraph has two layout engines: dagre.js (default) and ELK.js (`--layout elk`). Both run via goja (Go JS runtime) and produce self-contained SVGs with base64-embedded cloud provider icons.

D2 and go-graphviz renderers have been removed.

## Completed

- [x] AWS Cloud boundary with official AWS Cloud icon
- [x] VPC/Subnet/SG container nesting with colored borders and icons
- [x] Resource nodes with real AWS/Azure/GCP Architecture Icons (base64-embedded)
- [x] Container z-ordering — outermost drawn first (back), innermost last (front)
- [x] Inferred flow edges (ALB→EC2, EC2→RDS, Lambda→S3, etc.) for AWS, Azure, GCP
- [x] Self-contained SVG — no external file references, portable
- [x] Dark mode — CSS @media (prefers-color-scheme: dark) auto-detection
- [x] ELK.js layout engine with orthogonal edge routing
- [x] ELK left-to-right direction — labels at top, edges enter from left, no overlap
- [x] Container minimum width constraint — labels no longer clipped
- [x] Azure YAML mappings with Resource Group → VNet → Subnet nesting
- [x] GCP YAML mappings with VPC → Subnet nesting
- [x] Official Azure/GCP container styling (brand colors)
- [x] D2 and go-graphviz renderers removed
- [x] Security group containment via vpc_security_group_ids
- [x] Count/for_each collapsing with badges

## Remaining

### Edge quality (ELK)
- [ ] S3/storage nodes outside VPC get awkward horizontal edges — consider separate layout zone for global services
- [ ] Multiple parallel edges to same target still overlap slightly

### Visual polish
- [ ] Azure/GCP cloud boundary logos (currently text only)
- [ ] Region/zone container support for Azure and GCP
- [ ] Icon quality at upscale (Azure icons 18x18 native, GCP 24x24)

### Dagre renderer
- [ ] Dagre kept as `--layout dagre` option but not actively improved
- [ ] Known limitation: B-spline edges can look wobbly on complex graphs

## Layout Engine Comparison

| Feature | dagre (default) | ELK (`--layout elk`) |
|---------|----------------|---------------------|
| Direction | Top-down | Left-to-right |
| Edge routing | B-spline curves | Orthogonal (right-angle) |
| Edge-label overlap | Yes (known issue) | No (labels at top, edges from left) |
| Container min-width | No | Yes (enforced for labels) |
| Binary size overhead | 278KB | 3.6MB |
| Edge quality | Smooth but wobbly | Clean straight lines |
