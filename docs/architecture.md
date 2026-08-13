# Architecture

## Overview

stackgraph is a standalone Go binary that generates production-quality infrastructure diagrams from OpenTofu/Terraform state files, plan JSON, HCL source code, or DOT graph output.

```
Input (state/plan/HCL/DOT)
  → Parse into internal graph (nodes + edges)
  → ApplyMappings (service, icon, style, containment rules from YAML registry)
  → DetectImplicitEdges (id/ARN/list reference scanning)
  → ApplyGrouping (parent_ref containment; edge fallback; SG containment; cloud boundary)
  → ApplyMappings again (styles the synthesized cloud-boundary nodes)
  → InferFlowEdges (opt-in --infer-flows, scoped to one network container)
  → CollapseCountInstances (containment-preserving)
  → FilterInternal
  → Render (JSON, DOT, SVG, HTML)
```

The ordering is deliberate: reference edges must exist before grouping so the
edge-based containment fallback (used by DOT/HCL inputs, which carry no
attributes) has real edges to work with. State/plan inputs get deterministic
containment from `parent_ref` attribute lookups and don't depend on edges at
all.

## Rendering

One layout engine: **ELK.js** (Eclipse Layout Kernel, 3.6MB JS bundle) executed
in-process via goja (Go JS runtime). No system dependencies, fully offline.

- Orthogonal (right-angle) edge routing, hierarchical compound layout
- The goja VM is created once per process and reused (the bundle parse costs
  ~300ms); a 30s interrupt timeout guarantees a pathological layout errors out
  instead of hanging
- Containers render back-to-front with label icons; edges render between
  container backgrounds and labels; leaf nodes render on top
- Icons are base64-embedded — SVG output is fully self-contained
- Dark mode via `prefers-color-scheme` CSS inside the SVG

Output formats:

| Format | Purpose |
|--------|---------|
| `svg`  | Static, self-contained, embeddable in docs/CI |
| `html` | Interactive viewer: pan/zoom, click-to-inspect side panel, search filter, dark mode — single file, no external requests |
| `json` | Graph contract for frontends (Stackweaver workspace tab renders it with @xyflow/react) |
| `dot`  | Graphviz interchange |

## Package Structure

```
stackgraph/
├── cmd/stackgraph/              # CLI entrypoint (cobra)
│   ├── main.go                  # Root command, version wiring
│   ├── generate.go              # generate command, pipeline, input auto-detection
│   ├── mappings.go              # mappings list + mappings coverage commands
│   └── e2e_test.go              # end-to-end CLI tests for every input mode
├── pkg/
│   ├── parser/                  # Input parsers
│   │   ├── state.go             # terraform-json State parsing (addresses kept as-is)
│   │   ├── plan.go              # Plan parsing: actions incl. replace, destroyed resources
│   │   ├── hcl.go               # HCL source parsing via hashicorp/hcl/v2
│   │   ├── dot.go               # DOT graph parsing via gographviz
│   │   └── terragrunt.go        # terragrunt.hcl dependency parsing (type-safe)
│   ├── graph/                   # Internal graph representation
│   │   ├── graph.go             # Node, Edge, Graph, mappings application, collapse
│   │   └── grouping.go          # parent_ref containment, ref index, edge detection,
│   │                            #   SG containment, flow inference, cloud boundary
│   ├── mapping/                 # Resource type → service/icon/style/containment
│   │   ├── types.go             # YAML schema (incl. StyleMapping, parent_ref)
│   │   ├── registry.go          # Loader, Lookup, List, LoadDir overlay
│   │   ├── icons_test.go        # every icon path validated against embedded set
│   │   └── mappings/            # Embedded YAML (aws.yaml, azure.yaml, gcp.yaml)
│   ├── icons/                   # Embedded cloud provider icon assets
│   └── output/                  # Renderers
│       ├── json.go              # Graph JSON (frontend contract)
│       ├── dot.go               # Graphviz DOT
│       ├── svg.go               # Shared SVG building blocks (nodes, containers, styles)
│       ├── svg_elk.go           # ELK layout via goja + SVG assembly
│       ├── html.go              # Interactive HTML viewer wrapping the SVG
│       └── layout_invariants_test.go  # child-inside-parent bbox checks on real layouts
├── testdata/                    # Fixtures: state (incl. two-vpc-realistic, nested
│                                #   modules), plan, terragrunt (incl. panic regression),
│                                #   hcl, dot, invalid
└── docs/
```

## Input Modes

### 1. State JSON (primary)

Parses `tofu show -json` output using `hashicorp/terraform-json`. Resource
addresses are used exactly as terraform-json reports them (fully qualified
inside child modules — never re-prefixed). Attributes are curated per resource
type; reference-shaped attributes (`*_id`, `*_ids`, `*_name`, `network`,
`subnets`, …) are always kept because containment and edge detection resolve
through them.

### 2. Plan JSON

Same as state via `planned_values`, plus action annotations from
`resource_changes`: create, update, **replace**, delete, no-op. Resources being
destroyed appear from the change's `before` values. Actions render as colored
accents on both icon nodes and box nodes.

### 3. HCL Source

Parses `.tf` files with `hashicorp/hcl/v2`, detecting cross-references via
`attr.Expr.Variables()`. Shows "what's defined", not "what's deployed".
Containment comes from the edge-based fallback.

### 4. DOT Graph

Parses `tofu graph` output via gographviz; cleans terraform prefixes and
filters provider/root nodes.

### 5. Terragrunt

Walks a tree for `terragrunt.hcl` files and builds a module-level dependency
graph from `dependency` blocks. Non-string `config_path` values are skipped
(never panic); edges to nonexistent modules are dropped.

## Containment Model

Containment is **declarative data**, not inference. Each mapping may declare:

- `is_group` / `group_level` — renders as a container at a nesting depth
- `group_parent` — the expected parent resource *type*
- `parent_ref` — the *attribute* holding the parent's id (or name)

Resolution builds an index of every node by `id`, `arn`, `self_link`, and
(type, `name`), then parents each node by direct attribute lookup — scalar or
list values. A name match is only accepted for the declared `group_parent` type
(names collide across types; ids don't). This is deterministic, works with zero
`depends_on`, and disambiguates any number of VPCs.

Fallbacks, in order: edge-based placement (DOT/HCL inputs), security-group
containment via `vpc_security_group_ids`, and a synthesized per-provider cloud
boundary. Edges that merely restate containment are suppressed. Count/for_each
collapse preserves the members' parent and rewrites all links.

## Edge Model

1. `depends_on` edges from the state.
2. Implicit reference edges: any curated attribute (scalar or list of strings)
   whose value equals another node's id — typed by attribute name.
3. Optional `--infer-flows`: heuristic type-pair flows (ALB→EC2, EC2→RDS, …),
   connected **only within the same network container** so multi-environment
   states never get invented cross-environment traffic.

Edges are deduplicated per source→target pair regardless of type.

## Dependencies

| Library | License | Purpose |
|---------|---------|---------|
| hashicorp/terraform-json | MPL-2.0 | State + plan JSON structs |
| hashicorp/hcl/v2 | MPL-2.0 | HCL + terragrunt parsing |
| zclconf/go-cty | MIT | HCL value typing (safe config_path handling) |
| gographviz | Apache-2.0 | DOT parsing/generation |
| dop251/goja | MIT | In-process JS runtime for ELK.js |
| spf13/cobra | Apache-2.0 | CLI framework |
| gopkg.in/yaml.v3 | MIT | Mapping YAML |

**We do NOT import `hashicorp/terraform`** — `terraform-json` provides stable,
versioned structs independent of the terraform version.

## Test Strategy

- **Structural invariants on real layouts** (pkg/output/layout_invariants_test.go):
  for six fixtures including a realistic two-VPC state, every child's bounding
  box must sit inside its parent's, no group renders empty, every node gets a
  position. These invariants are exactly what the original prototype violated.
- **End-to-end CLI tests** for every input mode and output format.
- **Mapping validation**: every YAML icon path must exist in the embedded set;
  category vocabulary is kept consistent.
- **Regression fixtures**: nested `child_modules` addresses, count-collapse
  containment, name-match type filtering, terragrunt non-string `config_path`.

## Remaining Work

- Evidence-based flow edges (target-group attachments, SG rule pairs, IAM ARNs)
- Global-services partition band in the layout (Route53/IAM/S3 cluster)
- Node consolidation rules (merge Route53 records, S3 bucket sub-resources)
- Icon pruning to the used set (binary size)
- Region/zone containers for Azure/GCP
- Optional server mode (`stackgraph serve --port 8080`)
