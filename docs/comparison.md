# Comparison to Existing Tools

## Why stackgraph Exists

Every existing IaC visualization tool has fundamental architectural problems:

### Terravision (Python, AGPL-3.0)

Best output quality — proper cloud icons, VPC/subnet nesting. But:

- **AGPL-3.0 license** — incompatible with BSL 1.1 for platform integration
- **Requires `terraform init` + credentials** — runs `terraform plan -refresh=false` under the hood
- **Fragile variable resolution** — reimplements a half-broken Terraform expression evaluator in Python with 50-depth recursion, mock data replacements, no `for` loop support. Source of most bugs (issues #184, #121, #115, #109, #100, #93)
- **Graphviz pipeline** — `neato` engine + `gvpr` post-processing scripts. Requires system Graphviz install
- **Python dependency hell** — `python-hcl2`, `pygraphviz` (C bindings break on macOS), `graphviz`, `boto3`

### InfraMap (Go, MIT)

- **Imports `hashicorp/terraform` internal packages** pinned to v0.15.3 (2021). Massive dependency, cannot parse newer state formats
- **Bare-bones output** — Graphviz DOT with a handful of icons. No grouping, no nesting, no interactivity
- **Slow maintenance** — core dependency permanently stuck on old Terraform

### Rover (Go, MIT)

- Requires `terraform init` + `terraform plan` to generate input
- No cloud-aware grouping — flat graph nodes
- Effectively abandoned (~2023)

### Blast Radius / Pluralith

Both dead (2020 and 2023). Pluralith requires SaaS.

## How stackgraph Is Different

| Aspect | Terravision | InfraMap | stackgraph |
|--------|-------------|----------|------------|
| **Input** | Requires `terraform init` + credentials | Imports terraform internals | Consumes `tofu show -json` — stable JSON API |
| **Dependencies** | Python + pygraphviz + Graphviz | hashicorp/terraform v0.15.3 | hashicorp/terraform-json (lightweight) |
| **Icons** | PNGs (533 AWS, 808 Azure, 47 GCP) | ~10 PNGs | SVGs from official cloud provider sets |
| **Grouping** | VPC/subnet nesting (good) | None (flat) | Full hierarchy via declarative YAML |
| **Provider support** | AWS, GCP, Azure | AWS, GCP, Azure, OpenStack | Any — generic fallback |
| **OpenTofu** | No | No | First-class |
| **Terragrunt** | Via CLI wrapper | No | Native HCL parsing |
| **License** | AGPL-3.0 | MIT | Apache-2.0 |

## The Key Insight

**Don't reimplement Terraform's evaluator — consume its output.**

The state/plan JSON from `tofu show -json` is a stable, well-documented contract. It contains fully resolved resource attributes, expanded `count`/`for_each` instances, and explicit dependency information. This eliminates the entire class of bugs that Terravision fights (variable resolution, expression evaluation, module fetching, provider schema loading).
