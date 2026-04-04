---
description: "Spec for migrating from dagre.js to ELK.js for layout engine — enabling orthogonal edge routing and better compound graph support"
status: proposed
status_description: "Proposed — dagre B-spline fix is interim, ELK.js is the proper solution for edge routing"
author: Michiel VH
goal: "Professional orthogonal edge routing and improved compound graph layout"
priority: medium
created: 2026-04-04
---

# Plan: ELK.js Layout Engine Migration

## Context

stackgraph currently uses dagre.js (278KB) for graph layout via goja (Go JS runtime). Dagre handles compound graphs and hierarchical layout well, but has a fundamental limitation: **no edge routing**. Dagre computes edge waypoints as dummy-node positions at each rank layer, resulting in diagonal line segments that cross through containers and create visual clutter.

The current workaround (cubic B-spline smoothing) reduces jitter but doesn't solve the underlying problem — edges still cross through containers because dagre has no obstacle avoidance.

## Why ELK.js

ELK (Eclipse Layout Kernel) is a Java-based layout engine compiled to JavaScript. It's the industry standard for hierarchical graph layout with proper edge routing.

| Feature | dagre.js | ELK.js |
|---------|----------|--------|
| Edge routing | None (dummy-node waypoints) | ORTHOGONAL, POLYLINE, SPLINES |
| Compound graphs | Basic (setParent) | Full hierarchical with port constraints |
| Edge-container interaction | Edges cross through containers | Edges route around containers |
| Parallel edge separation | Minimal (edgesep=20) | Full separation with configurable spacing |
| Size | 278KB | 3.6MB |
| License | MIT | EPL-2.0 |
| Used by | dagre-d3 | D2, Eclipse, many commercial tools |

**D2 uses ELK.js via goja** — same exact pattern we use with dagre. The migration path is proven.

## Scope

**In scope:**
- Replace dagre.js with elkjs in the goja runtime
- Configure ELK for orthogonal edge routing (`elk.edgeRouting: "ORTHOGONAL"`)
- Update SVG edge rendering to use ELK's computed edge routes
- Update node/container positioning from ELK's output
- Keep all existing features (icons, containers, dark mode, cloud boundaries)

**Out of scope:**
- Frontend rendering changes (ELK.js is used in the frontend too via `elkjs` npm package — that's already planned for the Stackweaver integration)
- Adding new resource types or icon mappings
- Changing the graph model or parsing

## Design

### ELK Graph Input Format

ELK takes a JSON graph with nested `children` arrays (natural fit for compound graphs):

```json
{
  "id": "root",
  "layoutOptions": {
    "elk.algorithm": "layered",
    "elk.direction": "DOWN",
    "elk.edgeRouting": "ORTHOGONAL",
    "elk.spacing.nodeNode": 80,
    "elk.layered.spacing.nodeNodeBetweenLayers": 100,
    "elk.padding": "[top=40,left=40,bottom=40,right=40]",
    "elk.hierarchyHandling": "INCLUDE_CHILDREN"
  },
  "children": [
    {
      "id": "aws_cloud.boundary",
      "children": [
        {
          "id": "aws_vpc.main",
          "children": [
            {
              "id": "aws_subnet.public",
              "children": [
                {"id": "aws_instance.web", "width": 96, "height": 124}
              ]
            }
          ]
        }
      ]
    }
  ],
  "edges": [
    {"id": "e1", "sources": ["aws_lb.web"], "targets": ["aws_instance.web"]}
  ]
}
```

### ELK Output Format

After layout, ELK adds `x`, `y` to each node and `sections` with `bendPoints` to each edge:

```json
{
  "id": "aws_instance.web",
  "x": 150, "y": 200,
  "width": 96, "height": 124
}
```

Edge sections contain start/end points and bend points for orthogonal routing:

```json
{
  "id": "e1",
  "sections": [{
    "startPoint": {"x": 198, "y": 324},
    "endPoint": {"x": 198, "y": 400},
    "bendPoints": [
      {"x": 198, "y": 362},
      {"x": 300, "y": 362}
    ]
  }]
}
```

### Key ELK Layout Options

| Option | Value | Purpose |
|--------|-------|---------|
| `elk.algorithm` | `layered` | Hierarchical top-to-bottom layout |
| `elk.direction` | `DOWN` | Top-to-bottom flow |
| `elk.edgeRouting` | `ORTHOGONAL` | Right-angle edges (the main improvement) |
| `elk.hierarchyHandling` | `INCLUDE_CHILDREN` | Layout all levels at once |
| `elk.spacing.nodeNode` | `80` | Horizontal spacing |
| `elk.layered.spacing.nodeNodeBetweenLayers` | `100` | Vertical spacing |
| `elk.padding` | `[top=40,...]` | Container padding |
| `elk.layered.crossingMinimization.strategy` | `LAYER_SWEEP` | Reduce edge crossings |

### Implementation Approach

1. **Embed elkjs** — download `elk.bundled.js` (3.6MB) and embed via `go:embed`
2. **Build ELK graph JSON** from our internal graph model — natural mapping since ELK uses nested `children` arrays (same as our parent-child hierarchy)
3. **Run ELK via goja** — `var elk = new ELK(); elk.layout(graph).then(...)` — note ELK is promise-based, need goja promise handling
4. **Extract positions** from ELK's output — `node.x`, `node.y` for positions, `edge.sections[0].bendPoints` for edge routes
5. **Render SVG** — same SVG generation as now, but edge paths use orthogonal bend points instead of B-spline smoothed waypoints

### goja Promise Handling

ELK's `layout()` returns a Promise. D2 handles this in `lib/jsrunner/goja.go`:

```go
// Run ELK layout (returns Promise)
result, _ := vm.RunString(`elk.layout(graph)`)
promise := result.Export().(*goja.Promise)

// Wait for promise resolution
for promise.State() == goja.PromiseStatePending {
    runtime.Gosched()
}

// Get result
layoutResult := promise.Result().Export()
```

### SVG Edge Rendering with Orthogonal Points

ELK's orthogonal edge points are already right-angle segments — just connect them with straight lines. No smoothing needed:

```go
func renderElkEdge(b *strings.Builder, section ElkEdgeSection) {
    // Start point
    fmt.Fprintf(&d, "M %.0f,%.0f", section.StartPoint.X, section.StartPoint.Y)
    // Bend points (right-angle turns)
    for _, bp := range section.BendPoints {
        fmt.Fprintf(&d, " L %.0f,%.0f", bp.X, bp.Y)
    }
    // End point
    fmt.Fprintf(&d, " L %.0f,%.0f", section.EndPoint.X, section.EndPoint.Y)
}
```

## Acceptance Criteria

- [ ] ELK.js embedded via `go:embed` (elkjs npm package `elk.bundled.js`)
- [ ] Layout computation via goja with Promise handling
- [ ] Orthogonal edge routing (`elk.edgeRouting: "ORTHOGONAL"`) produces right-angle edges
- [ ] Edges route around container boundaries (no crossing through VPC/subnet)
- [ ] All existing features preserved: icons, containers, dark mode, cloud boundaries, flow edges
- [ ] All test topologies (AWS, Azure, GCP) render correctly
- [ ] SVG output is self-contained (same as current)
- [ ] Binary size increase acceptable (~3.6MB for ELK vs ~278KB for dagre)

## Implementation Tasks

- [ ] Download `elk.bundled.js` from npm and embed via `go:embed`
- [ ] Create `computeElkLayout()` function that builds ELK JSON graph from internal model
- [ ] Handle goja Promise for ELK's async layout API
- [ ] Parse ELK output — extract node positions and edge bend points
- [ ] Update `generateDagreSVG` to use ELK positions (rename to `generateSVG`)
- [ ] Update edge rendering — orthogonal points need straight lines, not curves
- [ ] Remove dagre.js embed and dagre-specific layout code
- [ ] Update dagre layout options: configure ELK spacing, padding, routing
- [ ] Test with all topologies (AWS VPC, multi-service, three-tier, Azure, GCP, Proxmox)
- [ ] Regenerate all example SVGs
- [ ] Update rendering-spec.md and architecture.md
- [ ] Run `go mod tidy` (no new Go deps — goja already present)

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Binary size +3.3MB | Moderate | 3.6MB is acceptable for a CLI with embedded icons (currently ~8MB with icons) |
| ELK.js goja parse time | ~500ms cold start | Cache goja VM across multiple renders if needed |
| goja Promise handling complexity | Low | D2 has working implementation to reference |
| EPL-2.0 license | Low | EPL-2.0 allows use as dependency, only copyleft if modifying ELK source |
| ELK compound graph differences from dagre | Low | ELK's compound support is better than dagre's — more features, not fewer |

## Timeline

This is a straightforward swap — same goja pattern, same SVG generation, better layout data. The main work is the ELK JSON graph builder and Promise handling. Estimated: 1 focused session.
