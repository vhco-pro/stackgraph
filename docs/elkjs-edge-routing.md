---
description: "ELK.js edge routing analysis — official docs review and solution for label overlap"
status: completed
status_description: "Fixed by switching layout direction to RIGHT (left-to-right). Labels at top, edges enter from left — no overlap."
author: Michiel VH
created: 2026-04-04
---

# ELK.js Edge Routing — Official Docs Review

## Official Documentation Findings

### Edge Routing (org.eclipse.elk.edgeRouting)

Values: `UNDEFINED`, `POLYLINE`, `ORTHOGONAL`, `SPLINES`

`ORTHOGONAL` is correct for architecture diagrams — horizontal/vertical segments only.

### Hierarchy Handling (org.eclipse.elk.hierarchyHandling)

Values: `INHERIT`, `INCLUDE_CHILDREN`, `SEPARATE_CHILDREN`

- `INCLUDE_CHILDREN` — lays out entire hierarchy in one pass, allows cross-hierarchical edge routing
- `SEPARATE_CHILDREN` — each container laid out independently

`INCLUDE_CHILDREN` is correct for our use case — we need edges between nodes in different containers.

### Port Constraints (org.eclipse.elk.portConstraints)

Values: `UNDEFINED`, `FREE`, `FIXED_SIDE`, `FIXED_ORDER`, `FIXED_RATIO`, `FIXED_POS`

- `UNDEFINED` (default) — ELK decides port positions and sides
- `FREE` — ports can be placed on any side freely
- `FIXED_SIDE` — port locked to a specific side, requires `elk.port.side` to be set

### Port Side (org.eclipse.elk.port.side)

Values: `UNDEFINED`, `NORTH`, `EAST`, `SOUTH`, `WEST`

Must be set when portConstraints is `FIXED_SIDE` or `FIXED_ORDER`.

### Edge Coordinates (org.eclipse.elk.json.edgeCoords)

Values: `CONTAINER` (default), `PARENT`, `ROOT`

- `CONTAINER` — edge points relative to edge's proper container (default)
- `PARENT` — edge points relative to the node where edge is defined
- `ROOT` — edge points are absolute (global coordinates)

**Use `ROOT` to avoid coordinate translation bugs.**

### Key Constraint

**The layered algorithm with `direction: DOWN` always routes edges top-to-bottom.** Edges enter compound nodes from the top and exit from the bottom. This is fundamental to the layered layout — there is no ELK option to change it.

When an edge enters a compound node from the top, it will always cross through the label area if the label is placed at the top. ELK's `padding` only affects where CHILDREN are placed inside the container, not where edges cross the container boundary.

## The Real Problem

ELK routes edges correctly — the orthogonal routing works, edges properly cross compound boundaries. The problem is purely visual: **our container labels are drawn at the top of the container, and edges entering from the top cross through the label text.**

ELK doesn't know about our labels because we draw them ourselves in SVG (not via ELK's label system). Even if we used ELK labels with `COMPUTE_PADDING`, edges would still enter from the top — they'd just enter below the label padding, which pushes everything down and creates the excess whitespace we saw earlier.

## Solution

**Change layout direction from DOWN to RIGHT.** With `elk.direction: "RIGHT"` (left-to-right flow):
- Edges enter containers from the LEFT side
- Labels are at the TOP of containers
- Edges and labels occupy different sides — no overlap possible

Additionally, set `elk.nodeSize.constraints: "[MINIMUM_SIZE]"` with `elk.nodeSize.minimum` on containers to ensure they're wide enough to fit their label text.

## Implementation (completed)

- [x] Changed `elk.direction` from `"DOWN"` to `"RIGHT"`
- [x] Added `elk.nodeSize.constraints: "[MINIMUM_SIZE]"` to containers
- [x] Added `elk.nodeSize.minimum` computed from label text length
- [x] Verified with AWS three-tier, multi-service, Azure, and GCP examples
- [x] Labels no longer overlap with edges
