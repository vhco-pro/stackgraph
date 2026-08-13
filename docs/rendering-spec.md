---
description: "Spec for stackgraph rendering quality — layout engine, edge routing, and visual polish"
status: in-progress
status_description: "ELK-only renderer, mapping-driven styling, interactive HTML viewer shipped. Remaining: global-services partition, region/zone containers."
author: Michiel VH
goal: "Production-quality infrastructure diagrams with embedded icons, nested containers, auto dark mode, and an interactive viewer"
priority: high
created: 2026-04-03
---

# Plan: Rendering Quality

## Context

stackgraph renders with a single layout engine: ELK.js via goja. Styling (icons,
container colors, dark-mode roles) is driven entirely by the YAML mapping
registry — the renderer holds no per-provider knowledge. dagre, D2, and
go-graphviz renderers have all been removed.

## Completed

- [x] AWS/Azure cloud boundary with official icons; brand-styled containers
- [x] VPC/Subnet/SG container nesting with colored borders and label icons
- [x] Deterministic containment from parent_ref (multi-VPC states nest correctly)
- [x] Evidence-based edges; heuristic flows opt-in and VPC-scoped
- [x] Count/for_each collapsing with badges, containment-preserving
- [x] Container z-ordering — outermost drawn first, labels above edges
- [x] Self-contained SVG — icons base64-embedded, portable
- [x] Dark mode — CSS prefers-color-scheme; style roles from YAML (cloud/vpc/subnet/sg)
- [x] Orthogonal edge routing; all edge sections rendered; arrowhead on last section
- [x] Box nodes sized to their labels — no truncation, rune-safe
- [x] Plan-action accents on icon and box nodes (create/update/replace/delete)
- [x] Interactive HTML viewer (--format html): pan/zoom, click-to-inspect, search, dark mode
- [x] goja hardening: cached VM, 30s interrupt timeout
- [x] Layout invariants enforced by tests (child-inside-parent, no empty containers)

## Remaining

### Layout
- [ ] Global-services partition: nodes outside any VPC (Route53, IAM, S3, ...) currently
      scatter in ELK's general flow; give them an explicit partition band
- [ ] Parallel edges to the same target can still run close together
- [ ] Region/zone containers for Azure/GCP

### Visual polish
- [ ] GCP cloud boundary logo (no suitable icon in the embedded set)
- [ ] Icon quality at upscale for small source SVGs
