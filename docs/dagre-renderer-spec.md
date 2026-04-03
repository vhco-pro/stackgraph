---
description: "Spec for a custom SVG renderer using dagre.js layout via goja (Go JS runtime) with base64-embedded cloud provider icons"
status: proposed
status_description: "Proposed as 3rd renderer option alongside D2 and Graphviz — build and compare"
author: Michiel VH
created: 2026-04-03
---

# Plan: Custom Dagre Renderer

## Context

stackgraph has two SVG renderers, both with fundamental issues:

- **D2** — icons reference temp file paths (`href="/tmp/..."`) instead of base64-embedding. SVG only works on the generating machine. Also uses its own visual style (not Terravision-like).
- **go-graphviz WASM** — WASM sandbox cannot render images at all. Colored containers work but icons are silently stripped.

Both issues stem from the same root cause: we're fighting third-party rendering libraries that weren't designed for our use case (cloud architecture diagrams with embedded icons in self-contained SVGs).

## Approach

Build a custom SVG renderer that uses **dagre.js** for layout computation (via **goja**, Go's embedded JS runtime) and generates SVG directly with full control over icon embedding.

This is not theoretical — **D2 itself uses this exact pattern in production**. D2 embeds dagre.js (278KB) via `go:embed`, runs it through goja to compute node positions, then generates SVG. We're doing the same thing but with our own SVG generation that supports base64-embedded cloud provider icons.

```
Graph JSON
    ↓
dagre.js (278KB, go:embed)
    ↓  (via goja JS runtime)
Node positions {x, y, w, h} + edge routes
    ↓
Custom SVG generator
    ↓  reads icons from go:embed, base64-encodes inline
Self-contained SVG with embedded icons
```

## Why This Works

| Requirement | dagre.js + custom SVG |
|---|---|
| Single binary | Yes — goja is pure Go, dagre.js embedded at 278KB |
| Cloud provider icons | Yes — base64-encoded `<image href="data:image/png;base64,...">` |
| Self-contained SVG | Yes — no external file references |
| Nested containers | Yes — dagre supports `setParent()` for compound graphs |
| Professional layout | Yes — hierarchical DAG layout, same algorithm as D2 |
| No system deps | Yes — no Graphviz install needed |
| Production-proven | Yes — D2 uses identical goja + dagre pattern |

## Design

### Package: `pkg/output/svg_dagre.go`

```go
func RenderDagreSVG(g *graph.Graph) ([]byte, error) {
    // 1. Run dagre.js via goja to get positions
    positions, err := computeDagreLayout(g)

    // 2. Generate SVG with embedded icons
    svg := generateSVG(g, positions)

    return svg, nil
}
```

### Layout Computation (dagre.js via goja)

Embed dagre.js and run it via goja:

```go
//go:embed dagre.min.js
var dagreJS string

func computeDagreLayout(g *graph.Graph) (map[string]NodePosition, error) {
    vm := goja.New()
    vm.RunString(dagreJS)

    // Create dagre graph with compound support
    vm.RunString(`var g = new dagre.graphlib.Graph({compound: true})`)
    vm.RunString(`g.setGraph({rankdir: "TB", nodesep: 80, ranksep: 100, marginx: 40, marginy: 40})`)
    vm.RunString(`g.setDefaultEdgeLabel(function() { return {} })`)

    // Add nodes with sizes
    for _, n := range g.Nodes {
        w, h := nodeSize(n) // groups are larger
        vm.RunString(fmt.Sprintf(`g.setNode(%q, {width: %d, height: %d, label: %q})`, n.ID, w, h, n.Label))
    }

    // Set parent relationships for nesting
    for _, n := range g.Nodes {
        if n.Parent != "" {
            vm.RunString(fmt.Sprintf(`g.setParent(%q, %q)`, n.ID, n.Parent))
        }
    }

    // Add edges
    for _, e := range g.Edges {
        vm.RunString(fmt.Sprintf(`g.setEdge(%q, %q)`, e.Source, e.Target))
    }

    // Run layout
    vm.RunString(`dagre.layout(g)`)

    // Extract positions
    // g.nodes() returns IDs, g.node(id) returns {x, y, width, height}
}
```

### SVG Generation

Full control over output — no third-party SVG library:

```xml
<svg xmlns="http://www.w3.org/2000/svg" width="W" height="H">
  <!-- Container: VPC -->
  <rect x="10" y="10" width="500" height="400" rx="8"
        fill="#F8F4FF" stroke="#8C4FFF" stroke-width="2"/>
  <text x="20" y="30" font-family="Sans-Serif" fill="#6B21A8">VPC — main</text>

  <!-- Container: Subnet (nested inside VPC) -->
  <rect x="30" y="50" width="200" height="300" rx="8"
        fill="#F2F7EE" stroke="#7CB342" stroke-width="2"/>

  <!-- Resource node: EC2 with embedded icon -->
  <image href="data:image/png;base64,iVBOR..." x="50" y="80" width="64" height="64"/>
  <text x="82" y="160" text-anchor="middle" font-family="Sans-Serif"
        font-size="12" fill="#2D3436">web_a</text>

  <!-- Edge: curved path -->
  <path d="M 82,170 C 82,200 200,200 200,230"
        fill="none" stroke="#7B8894" stroke-width="1.5"
        marker-end="url(#arrowhead)"/>
</svg>
```

### Visual Parameters (Terravision-inspired)

| Element | Attribute | Value |
|---------|-----------|-------|
| **Canvas** | background | `#FFFFFF` |
| **Canvas** | font | `Sans-Serif` |
| **Resource node** | icon size | 64x64 px |
| **Resource node** | label position | below icon, centered |
| **Resource node** | label font | Sans-Serif, 12px, `#2D3436` |
| **Resource node** | total height | icon (64) + gap (8) + label (16) = ~88px |
| **Group: VPC** | fill | `#F8F4FF`, stroke `#8C4FFF`, 2px solid |
| **Group: Subnet public** | fill | `#F2F7EE`, stroke `#7CB342` |
| **Group: Subnet private** | fill | `#DEEBF7`, stroke `#7CB342` |
| **Group: Security Group** | fill | `#FFF5F5`, stroke `#E53935`, dashed |
| **Group: generic** | fill | `#F8F9FA`, stroke `#ADB5BD`, dashed |
| **Edge** | stroke | `#7B8894`, 1.5px |
| **Edge** | arrowhead | standard triangle |
| **Layout** | direction | top-to-bottom |
| **Layout** | node separation | 80px horizontal |
| **Layout** | rank separation | 100px vertical |
| **Layout** | margin | 40px |

### Icon Embedding

```go
func embedIcon(iconPath string) string {
    data, err := icons.GetIconBytes(iconPath)
    if err != nil {
        return "" // no icon, render as labeled box instead
    }

    ext := filepath.Ext(iconPath)
    mime := "image/png"
    if ext == ".svg" {
        mime = "image/svg+xml"
    }

    return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
}
```

### Dagre Compound Graph Handling

Dagre's `setParent()` creates compound nodes. When layout runs:
- Parent nodes are sized to contain their children (with padding)
- Children are positioned inside their parent's bounding box
- Edges route between nodes, crossing container boundaries as needed

This eliminates the Graphviz duplicate-node bug entirely — containers are not subgraph clusters, they're compound graph parents.

### Edge Routing

Dagre computes edge points as arrays of `{x, y}` coordinates. We render them as SVG `<path>` elements using cubic bezier curves:

```go
func edgePath(points []Point) string {
    if len(points) < 2 {
        return ""
    }
    // Start point
    d := fmt.Sprintf("M %d %d", points[0].X, points[0].Y)
    // Cubic bezier through remaining points
    for i := 1; i < len(points); i++ {
        d += fmt.Sprintf(" L %d %d", points[i].X, points[i].Y)
    }
    return d
}
```

## Acceptance Criteria

- [ ] `stackgraph generate --input state.json -f svg --renderer dagre` produces a self-contained SVG
- [ ] SVG contains base64-embedded cloud provider icons (no external file references)
- [ ] VPC, Subnet, Security Group render as colored container rectangles
- [ ] EC2 instances render with the official AWS EC2 icon
- [ ] Resource nodes without icons render as clean labeled boxes
- [ ] Edges render with arrowheads and route correctly between containers
- [ ] Count badge (x3) visible on collapsed nodes
- [ ] SVG works when opened on any machine (fully portable)
- [ ] No system dependencies (no Graphviz, no headless browser)

## Implementation Tasks

- [ ] Download dagre.min.js and embed via `go:embed`
- [ ] Create `pkg/output/svg_dagre.go` with layout computation via goja
- [ ] Implement compound graph setup (setParent for VPC/subnet nesting)
- [ ] Implement SVG generation with base64 icon embedding
- [ ] Container rendering with Terravision color scheme
- [ ] Edge path rendering with arrowhead markers
- [ ] Count badge rendering
- [ ] Fallback to labeled box for unmapped resources
- [ ] Wire up `--renderer dagre` flag in CLI
- [ ] Unit test: SVG contains base64 `data:image/png` references
- [ ] Unit test: SVG has no external file references
- [ ] Generate comparison examples alongside D2 and Graphviz output

## Dependencies

| Package | Purpose | Already in go.mod? |
|---------|---------|-------------------|
| `github.com/dop251/goja` | Go JS runtime | Yes (D2 transitive dep) |
| dagre.min.js (278KB) | Graph layout | Embed via `go:embed` |

No new Go dependencies needed — goja is already pulled in via D2.

## Risks

| Risk | Mitigation |
|------|------------|
| dagre.js compound layout quality | dagre is battle-tested, used by D2 in production. If insufficient, upgrade to ELK.js (3.6MB, same goja pattern) |
| SVG rendering edge cases (text overflow, long labels) | Truncate labels at 24 chars, use title attribute for full text |
| goja performance for large graphs | dagre.js parse takes ~100-500ms, layout is fast. Acceptable for CLI |