# Architecture

## Overview

stackgraph is a standalone Go binary that generates production-quality infrastructure diagrams from OpenTofu/Terraform state files, plan JSON, HCL source code, or DOT graph output.

```
Input (state/plan/HCL/DOT)
  → Parse into internal graph (nodes + edges)
  → Apply resource mappings (service name, icon, grouping rules)
  → Apply provider-specific grouping (VPC → Subnet → instances)
  → Detect implicit edges (vpc_id, subnet_id attribute scanning)
  → Collapse count/for_each instances into single nodes
  → Filter internal/meta nodes (providers, root)
  → Render output (JSON, DOT, SVG/PNG)
```

## Rendering Strategy

stackgraph produces two types of visual output that share the same graph JSON:

```
                    ┌─ CLI: dagre.js or ELK.js → static SVG (self-contained)
Graph JSON ─────────┤
                    └─ Stackweaver frontend: @xyflow/react + ELK.js → interactive diagram
```

**CLI static output** — Uses dagre.js (default) or ELK.js (`--layout elk`) via goja (Go JS runtime) for layout, then generates SVG directly with base64-embedded cloud provider icons. No system dependencies, fully offline, self-contained SVG output.

Two layout engines available:
- **dagre** (default, `--layout dagre`) — top-down layout with B-spline curve edges, 278KB
- **ELK** (`--layout elk`) — left-to-right layout with orthogonal (right-angle) edges, 3.6MB. Better edge routing, no edge-label overlap.

**Stackweaver frontend** — Consumes the same graph JSON via API and renders with `@xyflow/react` + ELK.js for an interactive experience (pan, zoom, click-to-inspect, search, export). Documented in the Stackweaver integration plan.

## Package Structure

```
stackgraph/
├── cmd/stackgraph/              # CLI entrypoint (cobra)
│   ├── main.go                  # Root command
│   ├── generate.go              # generate command + input auto-detection
│   └── mappings.go              # mappings list command
├── pkg/
│   ├── parser/                  # Input parsers
│   │   ├── state.go             # terraform-json State parsing
│   │   ├── plan.go              # terraform-json Plan parsing (with action annotations)
│   │   ├── hcl.go               # HCL source parsing via hashicorp/hcl/v2
│   │   ├── dot.go               # DOT graph parsing via gographviz
│   │   └── terragrunt.go        # terragrunt.hcl dependency parsing
│   ├── graph/                   # Internal graph representation
│   │   ├── graph.go             # Node, Edge, Graph types + count collapsing + filtering
│   │   └── grouping.go          # Provider-specific grouping + implicit edge detection
│   ├── mapping/                 # Resource type → cloud service mapping
│   │   ├── types.go             # ProviderMappings, ResourceMapping, VariantMapping
│   │   ├── registry.go          # Registry loader + Lookup/List
│   │   ├── embed.go             # go:embed for bundled YAML mappings
│   │   └── mappings/            # Embedded YAML files (aws.yaml, etc.)
│   └── output/                  # Output renderers
│       ├── json.go              # JSON graph data (for API/frontend consumption)
│       ├── dot.go               # Graphviz DOT text format
│       └── svg.go               # Static SVG/PNG via go-graphviz (Terravision-quality)
├── icons/                       # Cloud provider icon assets (PNG, embedded via go:embed)
│   ├── aws/                     # AWS Architecture Icons
│   ├── azure/                   # Azure Architecture Icons
│   ├── gcp/                     # GCP Icons
│   └── generic/                 # Generic fallback icons
├── testdata/                    # Mock state/plan/HCL/DOT files for unit tests
├── examples/                    # Generated example outputs (SVG, JSON, DOT)
├── docs/                        # This documentation
├── go.mod                       # github.com/michielvha/stackgraph, Go 1.26.1
├── README.md
└── LICENSE                      # Apache-2.0
```

## Input Modes

### 1. State JSON (primary)

Parses `tofu show -json` / `terraform show -json` output using `github.com/hashicorp/terraform-json` (`tfjson.State`). This is the most complete input mode — contains all resource attributes, `depends_on` relationships, and module hierarchy.

Attributes are curated per resource type to keep graph JSON small — only attributes useful for display (tooltips, variant detection, implicit edge detection) are kept.

### 2. Plan JSON

Parses plan output using `tfjson.Plan`. Same as state but adds `Action` annotations (create/update/delete/no-op) to each node from `resource_changes`.

### 3. HCL Source

Parses `.tf` files directly using `hashicorp/hcl/v2` with `hclsyntax`. Extracts resource blocks and detects cross-references via `attr.Expr.Variables()`. Does not require `tofu init` or credentials — shows "what's defined" not "what's deployed".

### 4. DOT Graph

Parses `tofu graph` output using `gographviz`. Cleans terraform-specific prefixes (`[root]`, `(expand)`, `(close)`), filters provider/root nodes, and extracts dependency edges.

### 5. Terragrunt

Walks a directory tree for `terragrunt.hcl` files, parses `dependency` blocks via HCL, and builds a module-level dependency graph.

## Graph Processing Pipeline

After parsing, the graph goes through these transformations in order:

1. **ApplyMappings** — enriches nodes with service/category/icon from YAML registry
2. **ApplyGrouping** — assigns parent-child relationships for group nodes (VPC → Subnet → instances)
3. **DetectImplicitEdges** — scans attribute values for ID references to other resources
4. **CollapseCountInstances** — merges `resource[0]`, `resource[1]`, `resource[2]` into one node with `count: 3`
5. **FilterInternal** — removes provider configuration nodes and terraform root nodes

## Resource Mapping System

Mappings are declarative YAML files loaded via `go:embed`. Each mapping specifies:

- `service` — human-readable service name (e.g., "EC2", "VPC")
- `category` — service category (e.g., "Compute", "Networking")
- `icon` — path to embedded icon file (e.g., `aws/compute/ec2-instance.png`)
- `is_group` — whether this resource type renders as a container (subgraph cluster)
- `group_level` — nesting depth (1 = outermost)
- `group_parent` — expected parent resource type
- `variants` — alternative icons based on attribute values

Unmapped resources render as generic nodes with the resource type as the label.

## SVG Rendering (CLI)

The static renderer uses dagre.js or ELK.js (via goja Go JS runtime) for layout computation, then generates SVG directly with full control over icon embedding, container styling, and dark mode.

### Rendering Approach

1. **Layout** — dagre.js (278KB) or ELK.js (3.6MB) computes node positions and edge routes via goja
2. **Containers** (VPC, Subnet, SG) render as `<rect>` elements with provider-specific colors and icons in labels
3. **Resource nodes** render with base64-embedded cloud provider icons (AWS PNG 64px, Azure/GCP SVG)
4. **Edges** render as orthogonal (ELK) or B-spline (dagre) `<path>` elements with arrowhead markers
5. **Dark mode** — CSS `@media (prefers-color-scheme: dark)` auto-swaps all colors
6. **Self-contained** — all icons base64-embedded, no external file references

### Visual Design

| Container | Fill | Stroke |
|-----------|------|--------|
| AWS VPC | `#F8F4FF` | `#8C4FFF` (purple) |
| AWS Subnet | `#F2F7EE` | `#7CB342` (green) |
| AWS Security Group | `#FFF5F5` | `#E53935` (red, dashed) |
| Azure Resource Group | `#F0F0F0` | `#767676` (gray, dashed) |
| Azure VNet | `#E7F4E4` | `#50E6FF` (cyan) |
| GCP VPC | `#E6F4EA` | `#34A853` (green) |
| GCP Subnetwork | `#F1F3F4` | `#5F6368` (gray) |

### Non-Cloud Providers

Providers without official icon sets (Proxmox, Kubernetes, Cloudflare, etc.) render as clean labeled boxes. Custom icons can be added by dropping files in `pkg/icons/icons/<provider>/` and adding entries to the YAML mapping.

## Dependencies

| Library | Version | License | Purpose |
|---------|---------|---------|---------|
| hashicorp/terraform-json | v0.27.2 | MPL-2.0 | State + plan JSON structs |
| hashicorp/hcl/v2 | v2.24.0 | MPL-2.0 | HCL source parsing |
| gographviz | v2.0.3 | Apache-2.0 | DOT format parsing/generation |
| dop251/goja | latest | MIT | Go JS runtime (runs dagre.js and ELK.js) |
| spf13/cobra | v1.10.2 | Apache-2.0 | CLI framework |
| gopkg.in/yaml.v3 | v3.0.1 | MIT | YAML mapping loading |

**We do NOT import `hashicorp/terraform`** (the full binary's internal packages). We use `hashicorp/terraform-json` which provides stable, versioned structs independent of the terraform version.

## Remaining Work

- [ ] Proxmox resource mappings YAML
- [ ] Azure/GCP region/zone container support
- [ ] Azure/GCP cloud boundary provider logos
- [ ] Plan JSON action annotations (create/update/delete visual diff)
- [ ] Terragrunt cross-module dependency visualization
- [ ] Variant detection (ALB vs NLB, Fargate vs EC2)
- [ ] Node consolidation rules (merge Route53 records, IAM policies)
- [ ] `stackgraph generate --format html` — self-contained interactive HTML
- [ ] Optional server mode (`stackgraph serve --port 8080`)
