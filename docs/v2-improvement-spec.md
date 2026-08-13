---
description: "Findings from the 2026-08 product review and the spec for making stackgraph correct at real-world scale — ID-based containment, evidence-based edges, mapping-driven rendering, single layout path"
status: in-progress
status_description: "Release 1 complete and most of Release 2: containment redesign, mappings as single source of truth (styles + icons + validation), dagre deleted, goja hardened, plan overhaul (replace/deletes/accents), terragrunt panic fix, invariant + e2e tests, mappings coverage tooling, --mappings overlay, interactive HTML viewer. Remaining: evidence-based flows, global-services partition, icon pruning, consolidation rules."
author: Michiel VH
goal: "Diagrams that stay correct and readable on real multi-VPC, multi-env states — not just single-VPC fixtures"
priority: high
created: 2026-08-13
---

# Plan: stackgraph v2 — Correct at Real-World Scale

## Why this exists

The POC produces good-looking diagrams on the bundled fixtures (single VPC,
5–10 resources, `depends_on` present) and falls apart on anything resembling a
real state. Reproduced with a synthetic 39-resource, two-VPC state
(`prod` + `staging`, no `depends_on` — which is what many real states look
like):

- **Containment collapsed entirely.** The VPC rendered as a small empty box;
  both subnets rendered as empty boxes *outside* the VPC; every EC2/RDS/SG
  floated free. Dozens of edges pointed at the walls of empty containers.
- **74 edges for 36 nodes**, most fabricated (see below), rendered as parallel
  spaghetti bundles crossing the full canvas.
- **Fabricated cross-environment flows**: prod instances → staging Redis,
  every instance → every S3 bucket.

Defects reproducible on the bundled fixtures themselves:

- `count`/`for_each` collapse **drops containment**: in
  `testdata/state/aws/count-foreach.json` the collapsed `EC2 web (x3)` node
  renders outside the VPC, pointing arrows at an empty subnet.
- Unmapped-but-common resources (`aws_internet_gateway`, `aws_nat_gateway`)
  render as icon-less grey boxes with truncated labels ("Internet Gateway — ...").
- Proxmox state renders as two bare disconnected boxes — no boundary, no edges.
- `testdata/plan/` and `testdata/terragrunt/` are **empty**: two of the five
  advertised input modes have zero fixtures and zero test coverage.

## Root causes (verified against the code)

### 1. Containment is edge-derived, and grouping runs before edge detection

`ApplyGrouping` (pkg/graph/grouping.go) assigns parents by asking "is there an
edge between this node and a group node of the expected type?" — but the
pipeline in cmd/stackgraph/generate.go:83-87 runs `ApplyGrouping()` **before**
`DetectImplicitEdges()`. At grouping time the only edges are `depends_on`,
which Terraform state mostly omits. No `depends_on` → no edges → no nesting,
silently. Even with edges present, edge-existence is the wrong predicate: with
two VPCs it cannot say *which* VPC a subnet belongs to (first match wins), and
it conflates "references" with "contains".

### 2. Nested-module addresses are duplicated

pkg/parser/state.go:52-55 prepends the module prefix to `r.Address`, but
terraform-json already returns fully-qualified addresses inside child modules,
yielding IDs like `module.net.module.net.aws_subnet.a`. Knock-on: plan action
lookup (plan.go:60) never matches module resources, `depends_on` targets never
resolve, count-collapse regexes and any address-keyed UI break. There is no
`child_modules` fixture anywhere, which is why this survives.

### 3. `inferFlowEdges` fabricates cartesian products

grouping.go:155-178 joins 40 hardcoded type pairs N×M with zero attribute
evidence. Measured: 30 instances + 30 DB instances → 900 synthetic edges, 0
real ones. Any real state becomes a hairball, and multi-env states get
invented cross-environment traffic. A diagram showing flows that don't exist
is worse than one showing none.

### 4. Collapse runs last and loses parents

`CollapseCountInstances` (graph.go:196-243) rewrites IDs and edges but not
`Parent`/`Children`. Stale child lists survive; both SVG renderers classify
containers via `len(n.Children) > 0`, so collapsed nodes fall out of their
clusters and empty containers render.

### 5. Plan mode is largely non-functional

plan.go:28-37 matches only exact single-action slices, so replace
(`["delete","create"]`) gets no action; nodes come only from `planned_values`,
so destroyed resources never appear and `ActionDelete` is unreachable; action
coloring exists only in the icon-less box branch of the dagre renderer, so
every mapped resource renders without plan color. No tests, no fixtures.

### 6. The YAML mapping layer is decorative

The renderers bypass the registry: icon choice is a hardcoded 51-type switch
(`getIconPath`, svg_dagre.go:558-663), container styling is three more
hardcoded maps, and 62 of the 91 YAML `icon:` paths point at files that don't
exist in `pkg/icons` (nothing notices because `Node.Icon` only reaches the
JSON output — where it hands the frontend broken paths). Provider knowledge is
spread across **six places**: YAML, three renderer switches,
`curateAttributes` in the state parser, and the flow/SG/boundary logic in
grouping.go. "Add a resource type with 3 lines of YAML" (README) is currently
false — it requires editing Go in several files.

### 7. Assorted correctness/robustness defects

- Terragrunt parser: tautological type check then unchecked `AsString()` —
  **panics** on non-string `config_path` (terragrunt.go:99-100); parse errors
  swallowed; edges emitted to nonexistent modules.
- Edge dedup key includes `Type`, so `depends_on` + `subnet_id` between the
  same pair render as duplicate overlapping arrows.
- goja VMs have no interrupt/timeout/memory bound (a layout bug hangs the CLI
  forever); ELK costs ~0.5s and ~140MB RSS per render; dagre is driven by one
  string-interpolated `RunString` per node/edge.
- Byte-slicing label truncation (`label[:21]`) produces invalid XML on
  multi-byte labels.
- Parent-chain walks and DOT subgraph recursion have no cycle guards.
- Unknown `--layout` silently falls back to dagre; `--terragrunt` silently
  ignored with `--input`; unrecognized JSON silently treated as state.
- The two SVG renderers are ~70% duplicated and already drifting (dark-mode
  CSS blocks differ).

## The v2 design

### Containment: declarative ID-based references (replaces edge-derived grouping)

Extend the YAML mapping schema so containment is data, not inference:

```yaml
aws_subnet:
  is_group: true
  group_level: 2
  parent_ref: vpc_id          # attribute holding the parent's id
aws_instance:
  parent_ref: subnet_id
aws_nat_gateway:
  parent_ref: subnet_id
azurerm_virtual_network:
  parent_ref_name: resource_group_name   # match by name attr, not id
```

Resolution: index every node by `id` (plus `arn`, `name`, `self_link`), then
parent each node by direct attribute lookup. Deterministic, works with zero
`depends_on`, disambiguates any number of VPCs/RGs, and eliminates the
O(nodes × groups × edges) `hasEdgeBetween` scans. Security-group containment
becomes a `parent_ref`-style rule on the same index instead of hardcoded Go.
A containment ref consumed this way is **not** emitted as a drawn edge.

### Edges: only what the state proves

1. `depends_on` edges — kept, rendered thin/muted.
2. Implicit reference edges — keep, but scan **lists and nested objects** too
   (today only scalar strings), index by id/arn/name, skip refs already used
   for containment, and dedup by node pair regardless of edge type.
3. Cartesian `inferFlowEdges` — **removed** from the default path. If kept,
   behind `--infer-flows`, same-VPC only, rendered dashed + labeled
   "inferred". Real flow evidence exists in the state and should be used
   instead: `aws_lb_target_group_attachment`, security-group rule
   source/target pairs, IAM policy resource ARNs, Lambda environment variables
   referencing queue/table/bucket ids.
4. Fan-outs above a threshold collapse to a single edge from the common
   ancestor container; multiplicity becomes a label.

### Pipeline order (generate.go)

```
Parse → ApplyMappings → BuildRefIndex → ApplyContainment →
DetectReferenceEdges → CollapseCountInstances (containment-preserving) →
FilterInternal → Render
```

Collapse must merge parent pointers, rewrite the parent's `Children`, and
re-point member edges to the collapsed node, deduped. Fix the module-prefix
duplication in the state parser (use `r.Address` as-is) with a `child_modules`
fixture proving it.

### Mappings become the single source of truth

One schema drives everything the renderer needs: icon path (validated at build
time against the embedded FS — a unit test walks every `icon:` and fails on
dangling paths), container style (fill/stroke/dash), label color, curated
attribute keep-list, `parent_ref`, variants, consolidation rules. Delete
`getIconPath` and the style switches; the renderer reads only the resolved
node. Then "3 lines of YAML" becomes true, and `--mappings <dir>` can load
user-supplied mappings at runtime without recompiling. Normalize the category
vocabulary (aws/gcp `Database` vs azure `Databases`) with a lint test.

### One layout engine, hardened

dagre output is strictly worse (single tall column, huge gaps, edges through
containers) and doubles every rendering fix. **Delete the dagre path; ELK is
the only engine.** The binary-size argument is void — the binary is 28MB, of
which 7.7MB is 1748 embedded icons (only ~51 ever used today; prune unused
icons in the same pass) and 3.9MB is JS.

goja hardening: reuse one VM per process, feed the graph via `vm.Set` (not
string-interpolated programs), set an interrupt with a `--layout-timeout`
(default 30s), and render all edge sections, not just `Sections[0]`.

Layout polish that stays on the list:
- Global-services band: nodes without a network parent (Route53, IAM, S3,
  DynamoDB, SQS…) go into an explicit partition instead of scattering.
- Disconnected nodes cluster together instead of floating.
- Label sizing: size boxes to labels, never ellipsize resource names
  (fixes "Internet Gateway — ..."), truncate by runes if ever needed.

### Plan mode that actually works

Union `planned_values` with `resource_changes` so deletes appear; map action
slices properly (`["delete","create"]` → replace); color/badge applies to
icon nodes too. This is a differentiator no competitor has — "what will this
apply do to my architecture" — and it must be real before being advertised.

### Test strategy that would have caught all of this

- Fixtures for every advertised mode — plan JSON, terragrunt (simple +
  panic-regression for non-string `config_path`), nested `child_modules`,
  multi-provider `mixed/`. Either ship fixtures + tests or cut the feature
  from the README until real.
- A large multi-VPC fixture (the synthetic prod+staging state from this
  review) as the canonical "realistic" test.
- **Structural invariants post-layout**, asserted against ELK output: every
  child bbox inside its parent bbox; no empty group boxes; no edge endpoint on
  a group node that has children; every fixture node mapped.
- Golden SVG tests for the small fixtures; an end-to-end `generate` test (none
  exists today — only the detect helpers are tested).
- Icon-path validation test (see mappings section); benchmark guardrails on
  the goja hot path.
- Delete the misleading `TestGenericFallback` (asserts its own constructor
  input; never calls `ApplyMappings`).

### Mapping coverage as a tracked metric

Current: AWS 62, Azure 16, GCP 13, Proxmox 0 — but effective renderer coverage
is the 51 hardcoded types. Sampling 35 common AWS types found 20 unmapped
(`aws_internet_gateway`, `aws_nat_gateway`, `aws_key_pair`,
`aws_lambda_permission`, `aws_iam_role_policy`, `aws_lb_listener_rule`, …).
Targets: AWS 150+, Azure 60+, GCP 60+. Add
`stackgraph mappings coverage --input state.json` printing mapped/unmapped
counts so gaps surface from real usage and unknowns turn into contributions.

## Product direction (beyond correctness)

The moat named in the README — consume `tofu show -json` instead of
re-implementing Terraform — is real and worth keeping. What makes this "the
best in the scene" is what competitors don't have:

1. **Interactive HTML output** (`--format html`): self-contained file
   embedding the graph JSON + a small viewer (pan/zoom, collapse containers,
   click for attributes, search). This, not static SVG, is the demo-able
   product; static SVG stays for CI/docs embedding.
2. **Stackweaver integration** (existing plan): the same graph JSON feeds
   @xyflow/react; the CLI viewer and the platform tab share one contract —
   freeze it with a JSON schema and a version field (and stop shipping broken
   icon paths through it).
3. **Plan-diff rendering** as above — unique in the space once functional.
4. **Graceful degradation** (already true, keep): unmapped types render as
   labeled boxes; never crash, never omit.

## Non-goals

- Reading HCL to *evaluate* expressions (stay on the parsed-output contract).
- Live cloud API scanning.
- Per-provider bespoke renderers — one renderer, data-driven styling.

## Implementation order

1. ~~Pipeline reorder + ID-based containment (`parent_ref`, ref index,
   containment resolver)~~ — **done** (fixes the flat-diagram class outright).
2. ~~Containment-preserving collapse; suppress containment edges; pair-keyed
   edge dedup; module-address fix with fixture~~ — **done**.
3. Cartesian `inferFlowEdges` — **removed from the default path; kept behind
   `--infer-flows`, scoped to a single network container**. Evidence-based
   flows (target-group attachments first) still to do. Implicit reference
   detection now also scans lists. ELK is the default layout; box labels are
   sized to fit and never truncate.
4. ~~Mappings as single source of truth (style schema, icon validation test,
   renderer switches deleted, `--mappings` runtime overlay)~~ — **done**;
   icon pruning still open.
5. ~~Delete dagre; harden goja (cached VM, interrupt/timeout, all edge
   sections)~~ — **done**; global-services partition still open.
6. ~~Structural-invariant tests on real layouts, end-to-end CLI tests for all
   modes, two-vpc-realistic fixture, plan / terragrunt fixtures incl.
   terragrunt panic fix~~ — **done**; golden SVGs deferred (invariants cover
   the failure class with less brittleness).
7. ~~Plan mode overhaul (replace actions, destroyed resources from
   `resource_changes`, action accents on icon nodes)~~ — **done**.
8. Mapping expansion (in progress) + ~~`mappings coverage`~~ — **done**.
9. ~~`--format html` interactive viewer (pan/zoom, click-to-inspect, search,
   dark mode; Playwright-verified)~~ — **done**.

Steps 1–6 are the "make it honest" release; 7–9 are the "make it win" release.
The 2026-08 results are documented with before/after renders in
[reviews/2026-08-product-review.md](reviews/2026-08-product-review.md).

## Follow-ups from the 2026-08 code review

Applied immediately: goja interrupt-race fix + VM discard on layout failure,
HTML panel escaping, deterministic ordering (collapse bases, boundary
providers, attribute scan), name-collision ambiguity in the ref index,
mixed-parent count groups stay expanded, straight-line fallback for edges ELK
returns no route for, label-preferring edge dedup, shared
`attrStringValues`/`loadRegistry`/`graph.Process` helpers, single-read
coverage command, `--layout` kept as a deprecated no-op, icon data-URI cache,
`parent_ref` attributes survive curation for `--mappings` overlays, generic
provider boundaries (any provider gets one; style via `<provider>_cloud`
mapping entry).

All previously-deferred design items have since landed as well:

- **Flow patterns in YAML** — done: `flows:` sections per provider file feed
  `InferFlowEdges` through the registry; `--mappings` users can declare their own.
- **Role/dark-mode vocabulary** — done: the role clamp is gone (any CSS-safe
  role name works), dark rules are generated from the styles the graph uses,
  and `dark_fill`/`dark_stroke`/`dark_label` override the built-in palette.
- **Member-list containment** — done: `member_ref:` on the container mapping
  generalizes the security-group special case; the hardcoded function is gone.
- **Network-scope declaration** — done: `network_scope: true` on VPC/VNet/
  network mappings drives flow scoping, with group-level 1 as fallback.
- **Icon `<defs>`/`<use>` dedup** — done: each icon embeds once per document
  (~35% smaller SVGs on the realistic fixture).
- **Shared palette constants** — done: one Go palette drives the SVG
  stylesheet and the HTML viewer chrome.
