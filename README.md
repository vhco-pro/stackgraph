# stackgraph

Infrastructure diagram generator for OpenTofu/Terraform. Produces correct, cloud-aware diagrams from state files, plan JSON, HCL source, or `tofu graph` output — as static SVG, interactive HTML, JSON graph data, or DOT.

## Features

- **Multiple input modes** — state JSON, plan JSON, HCL source, DOT graph, Terragrunt projects
- **OpenTofu-native** — first-class OpenTofu support, Terraform compatible
- **Correct containment** — declarative `parent_ref` attribute resolution nests resources into VPCs/subnets/security groups deterministically; works on multi-VPC, multi-environment states with no `depends_on` present
- **Evidence-based edges** — arrows come from actual references in the state (ids, ARNs, lists), never invented; optional `--infer-flows` adds heuristic flows scoped to a single network container
- **Cloud-aware** — official AWS/Azure/GCP icons and brand-styled containers, all declared in YAML mappings
- **Provider-agnostic** — unmapped resources render as clean labeled boxes (no crash, no omission); `stackgraph mappings coverage` shows what a given state needs
- **Extensible without recompiling** — `--mappings <dir>` overlays your own YAML mappings
- **Plan-aware** — create/update/replace/delete render as colored accents, including destroyed resources
- **Count/for_each collapsing** — `aws_instance.web[0..2]` becomes a single node with `x3` badge, still inside its subnet
- **Four output formats** — SVG (static, self-contained, auto dark mode), HTML (interactive: pan/zoom, click-to-inspect, search), JSON (for frontends), DOT (for Graphviz)

## Installation

```bash
go install github.com/vhco-pro/stackgraph/cmd/stackgraph@latest
```

## Usage

```bash
# From state file (most common)
tofu show -json > state.json
stackgraph generate --input state.json --format svg --output infra.svg

# Interactive HTML viewer (pan, zoom, click for attributes, search)
stackgraph generate --input state.json --format html --output infra.html

# From plan file (shows planned changes incl. deletions)
tofu show -json tfplan.bin > plan.json
stackgraph generate --input plan.json --format svg --output plan.svg

# From HCL source (no init or credentials needed)
stackgraph generate --source ./terraform/ --format svg --output infra.svg

# Pipe from tofu graph
tofu graph | stackgraph generate --format dot --output graph.dot

# Terragrunt project (module dependency graph)
stackgraph generate --source ./terragrunt-project/ --terragrunt --format json

# Add heuristic data-flow edges (ALB→EC2, EC2→RDS) within each VPC
stackgraph generate --input state.json --format svg --infer-flows --output infra.svg

# Check mapping coverage for your state / find unmapped types
stackgraph mappings coverage --input state.json

# Use your own mappings without recompiling
stackgraph generate --input state.json --mappings ./my-mappings/ --format svg

# List supported resource mappings
stackgraph mappings list --provider aws --category Compute
```

## How It Works

stackgraph consumes the JSON output that `tofu show -json` produces — a stable, well-documented contract containing fully resolved resource attributes, expanded `count`/`for_each` instances, and explicit dependency information.

```
Input (state/plan/HCL/DOT)
  → Parse into internal graph (nodes + edges)
  → Apply resource mappings (service, icon, style, containment rules)
  → Detect reference edges (id/ARN/list attribute scanning)
  → Resolve containment (parent_ref attribute lookups; edge fallback for DOT/HCL)
  → Collapse count/for_each instances (containment-preserving)
  → Filter internal/meta nodes
  → Render (SVG, HTML, JSON, DOT) via ELK.js orthogonal layout
```

## Resource Mappings

Mappings are declarative YAML files that fully drive rendering — service name, icon, containment, and container style:

```yaml
aws_instance:
  service: EC2
  category: Compute
  icon: aws/Arch_Compute/Arch_Amazon-EC2_64.png
  group_parent: aws_subnet
  parent_ref: subnet_id        # attribute that names the containing resource

aws_vpc:
  service: VPC
  category: Networking
  icon: aws/Arch_Networking-Content-Delivery/Arch_Amazon-Virtual-Private-Cloud_64.png
  is_group: true
  group_level: 1
  style: { fill: "#F8F4FF", stroke: "#8C4FFF", label: "#6B21A8", role: vpc }

aws_security_group:
  service: Security Group
  category: Security
  is_group: true
  group_level: 3
  member_ref: vpc_security_group_ids   # resources listing this SG become its children
  style: { fill: "#FFF5F5", stroke: "#E53935", label: "#C62828", role: sg, dashed: true }

aws_ecs_service:
  service: ECS
  category: Containers
  icon: aws/Arch_Containers/Arch_Amazon-Elastic-Container-Service_64.png
  variants:
    - match: { attribute: launch_type, value: FARGATE }
      icon: aws/Arch_Containers/Arch_AWS-Fargate_64.png
      service: Fargate

# Heuristic data-flow patterns for --infer-flows, scoped by network_scope containers
flows:
  - { from: aws_lb, to: aws_instance }
```

Containers can declare `network_scope: true` (VPC, VNet) — heuristic flows
never cross such a boundary — and `dark_fill`/`dark_stroke`/`dark_label` in
their style block to retune dark mode. All of this works from a `--mappings`
overlay directory too, without recompiling.

Adding a resource type really is a few lines of YAML — a test validates every icon path against the embedded icon set, and `--mappings` loads user mappings at runtime.

## Comparison to Existing Tools

| Aspect | Terravision | InfraMap | stackgraph |
|--------|-------------|----------|------------|
| **Input** | Requires `terraform init` + credentials | Imports terraform v0.15.3 internals | Consumes `tofu show -json` — stable JSON API |
| **Dependencies** | Python + pygraphviz + Graphviz | hashicorp/terraform (massive) | single static binary |
| **Containment** | VPC/subnet nesting | None (flat graph) | Deterministic attribute-based nesting via YAML |
| **Edges** | Heuristic | Raw dependency graph | Evidence from state; heuristics opt-in and scoped |
| **Plan diff** | No | No | create/update/replace/delete coloring |
| **Interactive output** | No | No | Self-contained HTML viewer |
| **OpenTofu** | No | No | First-class |
| **License** | AGPL-3.0 | MIT | Apache-2.0 |

The key architectural insight: **don't reimplement Terraform's evaluator — consume its output.**

## License

Apache-2.0
