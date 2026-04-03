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

## Dual Rendering Strategy

stackgraph produces two types of visual output that share the same graph JSON but render differently:

```
                    ┌─ CLI: go-graphviz → static SVG/PNG (Terravision-quality)
Graph JSON ─────────┤
                    └─ Stackweaver frontend: @xyflow/react + ELK.js → interactive diagram
```

**CLI static output** — Uses D2 (`oss.terrastruct.com/d2`, MPL-2.0) as a Go library to produce production-quality SVG diagrams with cloud provider icons embedded inline (base64), nested containers (VPC → Subnet → instances), and professional layout via the dagre engine. No system dependencies, fully offline. This is the standalone product — works without Stackweaver.

**Stackweaver frontend** — Consumes the same graph JSON via API and renders with `@xyflow/react` + ELK.js for an interactive experience (pan, zoom, click-to-inspect, search, export). This is the platform integration — documented in the Stackweaver integration plan.

These two renderers are completely independent. Changing the CLI renderer has zero impact on the frontend, and vice versa.

See [rendering-analysis.md](rendering-analysis.md) for the full D2 vs go-graphviz comparison and why D2 was chosen.

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

The static renderer uses D2 (`oss.terrastruct.com/d2`) as a Go library — purpose-built for architecture diagrams with first-class nested containers, automatic layout, and inline icon embedding.

### Rendering Approach

The renderer converts the internal graph to a D2 script programmatically:

1. **Resource nodes** render with cloud provider SVG icons embedded inline (base64) and labels below
2. **Group nodes** (VPC, Subnet, SG) render as D2 containers with styled borders, fill colors, and rounded corners
3. **Edges** render with automatic routing through container boundaries
4. **Layout** uses the dagre engine (hierarchical DAG layout, bundled in D2)
5. **Icons** are local SVG files from official cloud provider icon sets, bundled in the binary via `go:embed`. D2 base64-encodes them directly into the SVG — fully self-contained, no CDN, fully offline

### Visual Design Parameters

| Container | Fill | Stroke |
|-----------|------|--------|
| AWS VPC | `#F8F4FF` | `#8C4FFF` (purple) |
| AWS Subnet (public) | `#F2F7EE` | `#7CB342` (green) |
| AWS Subnet (private) | `#DEEBF7` | `#7CB342` (green) |
| AWS Security Group | `#FFF5F5` | `#E53935` (red) |
| Azure Resource Group | `#F0F8FF` | `#0078D4` (blue) |
| Azure VNet | `#E8F4FC` | `#0078D4` |
| GCP VPC | `#E3F2FD` | `#4285F4` |
| GCP Subnetwork | `#EDE7F6` | `#7C4DFF` |
| Generic | `#F8F9FA` | `#ADB5BD` |

Plan action colors: green (create), red (delete), amber (update).

### Non-Cloud Providers

Providers without official icon sets (Proxmox, Kubernetes, Cloudflare, DigitalOcean, Hetzner, etc.) render as clean labeled boxes with D2's theme styling (rounded borders, shadows). Custom SVG icons can be added by dropping files in `icons/<provider>/` and referencing them in the YAML mapping. D2 embeds them inline just like cloud provider icons.

See [rendering-analysis.md](rendering-analysis.md) for the full D2 vs go-graphviz comparison.

## Dependencies

| Library | Version | License | Purpose |
|---------|---------|---------|---------|
| hashicorp/terraform-json | v0.27.2 | MPL-2.0 | State + plan JSON structs |
| hashicorp/hcl/v2 | v2.24.0 | MPL-2.0 | HCL source parsing |
| gographviz | v2.0.3 | Apache-2.0 | DOT format parsing/generation |
| terrastruct/d2 | v0.7.1 | MPL-2.0 | SVG diagram rendering (layout, icons, themes) |
| spf13/cobra | v1.10.2 | Apache-2.0 | CLI framework |
| gopkg.in/yaml.v3 | v3.0.1 | MIT | YAML mapping loading |

**We do NOT import `hashicorp/terraform`** (the full binary's internal packages). This is intentional — InfraMap made this mistake and is permanently pinned to terraform v0.15.3. We use `hashicorp/terraform-json` which provides stable, versioned structs independent of the terraform version.

## Remaining Work

### Standalone (stackgraph-side)

- [ ] Download and embed cloud provider SVG icons locally in `icons/` via `go:embed`
- [ ] Switch `d2Icon()` from CDN URLs to local bundled icons
- [ ] Add generic fallback icon for unmapped resources
- [ ] Tune D2 container styling to match Terravision-quality output
- [ ] Azure resource mappings YAML (~80 resource types)
- [ ] GCP resource mappings YAML (~60 resource types)
- [ ] Proxmox resource mappings YAML (~10 resource types)
- [ ] Plan JSON action annotations (create/update/delete visual diff)
- [ ] Terragrunt cross-module dependency visualization
- [ ] Variant detection (ALB vs NLB, Fargate vs EC2)
- [ ] Node consolidation rules (merge Route53 records, IAM policies)
- [ ] `stackgraph generate --format html` — self-contained interactive HTML
- [ ] Optional server mode (`stackgraph serve --port 8080`)
