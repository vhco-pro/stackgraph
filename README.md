# stackgraph

Infrastructure diagram generator for OpenTofu/Terraform. Produces interactive, cloud-aware diagrams from state files, plan JSON, HCL source, or `tofu graph` output.

## Features

- **Multiple input modes** — state JSON, plan JSON, HCL source, DOT graph, Terragrunt projects
- **OpenTofu-native** — first-class OpenTofu support, Terraform compatible
- **Cloud-aware** — AWS resource mappings with service icons, VPC/subnet grouping
- **Provider-agnostic** — unmapped resources render as generic nodes (no crash, no omission)
- **Count/for_each collapsing** — `aws_instance.web[0..2]` becomes a single node with `x3` badge
- **Implicit edge detection** — scans resource attributes for ID references between resources
- **Multiple output formats** — JSON (for frontends), DOT (for Graphviz), SVG (for CLI)

## Installation

```bash
go install github.com/michielvha/stackgraph/cmd/stackgraph@latest
```

## Usage

```bash
# From state file (most common)
tofu show -json > state.json
stackgraph generate --input state.json --format svg --output infra.svg

# From plan file (shows planned changes)
tofu show -json tfplan.bin > plan.json
stackgraph generate --input plan.json --format json

# From HCL source (no init or credentials needed)
stackgraph generate --source ./terraform/ --format svg --output infra.svg

# Pipe from tofu graph
tofu graph | stackgraph generate --format dot --output graph.dot

# Terragrunt project (multi-module dependency graph)
stackgraph generate --source ./terragrunt-project/ --terragrunt --format json

# Auto-detect input format (state vs plan vs DOT)
stackgraph generate --input myfile.json --format svg

# List supported resource mappings
stackgraph mappings list --provider aws
stackgraph mappings list --provider aws --category Compute
```

## How It Works

stackgraph consumes the JSON output that `tofu show -json` produces — a stable, well-documented contract containing fully resolved resource attributes, expanded `count`/`for_each` instances, and explicit dependency information.

```
Input (state/plan/HCL/DOT)
  → Parse into internal graph (nodes + edges)
  → Apply resource mappings (service name, icon, grouping rules)
  → Apply provider-specific grouping (VPC → Subnet → instances)
  → Detect implicit edges (vpc_id, subnet_id attribute scanning)
  → Collapse count/for_each instances into single nodes
  → Filter internal/meta nodes (providers, root)
  → Render output (JSON, DOT, SVG)
```

## Resource Mappings

Mappings are declarative YAML files that map resource types to cloud service metadata:

```yaml
# AWS example
aws_instance:
  service: EC2
  category: Compute
  icon: aws/compute/ec2-instance.svg
  group_parent: aws_subnet

aws_vpc:
  service: VPC
  category: Networking
  icon: aws/networking/vpc.svg
  is_group: true
  group_level: 1

aws_ecs_service:
  service: ECS
  category: Containers
  icon: aws/containers/ecs-service.svg
  variants:
    - match: { attribute: launch_type, value: FARGATE }
      icon: aws/containers/fargate.svg
      service: Fargate
```

Adding support for a new resource type is just 3-4 lines of YAML.

### Current coverage

| Provider | Resources mapped |
|----------|-----------------|
| AWS      | 65+             |
| Azure    | Coming soon     |
| GCP      | Coming soon     |
| Proxmox  | Coming soon     |

## Comparison to Existing Tools

| Aspect | Terravision | InfraMap | stackgraph |
|--------|-------------|----------|------------|
| **Input** | Requires `terraform init` + credentials | Imports terraform v0.15.3 internals | Consumes `tofu show -json` — stable JSON API |
| **Dependencies** | Python + pygraphviz + Graphviz | hashicorp/terraform (massive) | hashicorp/terraform-json (lightweight) |
| **Icons** | PNGs | ~10 PNGs | SVGs from official cloud provider sets |
| **Grouping** | VPC/subnet nesting | None (flat graph) | Full hierarchy via declarative YAML |
| **Provider support** | AWS, GCP, Azure | AWS, GCP, Azure, OpenStack | Any — generic fallback for unmapped |
| **OpenTofu** | No | No | First-class |
| **Terragrunt** | Via CLI wrapper | No | Native HCL parsing |
| **License** | AGPL-3.0 | MIT | Apache-2.0 |

The key architectural insight: **don't reimplement Terraform's evaluator — consume its output.**

## License

Apache-2.0
