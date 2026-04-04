---
description: "Spec for fixing Azure and GCP diagram rendering to match official architecture diagram standards"
status: in-progress
status_description: "YAML mappings created, container nesting working. Icons render but need visual polish. Remaining: verify icon quality at scale, add region/zone containers."
author: Michiel VH
goal: "Azure and GCP diagrams that look like official architecture reference diagrams from Microsoft and Google"
priority: high
created: 2026-04-04
---

# Plan: Azure & GCP Diagram Quality

## Context

AWS diagrams are looking professional — correct icons, proper VPC/Subnet/SG nesting, AWS Cloud boundary. But Azure and GCP diagrams don't match their official architecture diagram standards:

**Azure issues:**
- Icons are wrong — VM shows generic monitor instead of Azure VM icon, AKS shows wrong icon
- No VNet/Subnet container nesting visible (containers exist but may not be rendering)
- No subscription boundary
- Should use Azure's light blue visual style with dashed boundaries
- Missing the "Microsoft Azure" logo in corner

**GCP issues:**
- No Project boundary
- No VPC/Subnet/Region/Zone container hierarchy
- Should use Google's brand color hierarchy (blue > green > yellow > red)
- Icons may be wrong or not rendering

## Reference

Azure official diagrams use:
- Light blue (#E5F0FB) subscription boundaries (dashed)
- Solid VNet boundaries with VNet icon in label
- Subnet containers inside VNet
- Resource Group as logical dashed boundary
- Microsoft Azure logo bottom-left

GCP official diagrams use:
- Blue project boundary (dashed)
- Green VPC boundary (solid)
- Yellow region boundary (dashed)
- Red/gray zone boundary (dotted)
- Google Cloud logo

## Acceptance Criteria

### Azure
- [ ] Azure VM icon renders as the correct Azure Virtual Machine icon (not a generic desktop)
- [ ] AKS icon renders as the correct Azure Kubernetes Service icon
- [ ] SQL Database icon renders correctly
- [ ] Storage Account icon renders correctly
- [ ] Virtual Network container renders with cyan border (#50E6FF)
- [ ] Subnet containers render inside VNet
- [ ] Resource Group container renders with gray dashed border
- [ ] Azure cloud boundary has Azure blue (#0078D4) border
- [ ] Azure cloud boundary label shows "Azure" with correct styling

### GCP
- [ ] Compute Engine icon renders correctly
- [ ] GKE icon renders correctly
- [ ] Cloud SQL icon renders correctly
- [ ] Cloud Storage icon renders correctly
- [ ] VPC Network container renders with green border (#34A853)
- [ ] Subnet containers render inside VPC
- [ ] GCP cloud boundary has Google Blue (#4285F4) border
- [ ] GCP cloud boundary label shows "Google Cloud"

## Implementation Tasks

### Completed
- [x] Verify all Azure icon file paths match actual files — all 8 paths verified
- [x] Verify all GCP icon file paths match actual files — all 6 paths verified
- [x] Create `azure.yaml` mapping file with VNet, Subnet, RG as group nodes
- [x] Create `gcp.yaml` mapping file with VPC, Subnet as group nodes
- [x] Azure Resource Group → VNet → Subnet → resources nesting working
- [x] GCP VPC → Subnet → resources nesting working
- [x] Azure cloud boundary auto-generated with correct styling (#0078D4)
- [x] GCP cloud boundary auto-generated with correct styling (#4285F4)
- [x] Container label icons for VNet, Subnet, RG, VPC
- [x] Test with both dagre and ELK layout engines
- [x] Regenerate Azure and GCP examples

### Remaining
- [ ] Azure: add region/zone container support
- [ ] GCP: add region/zone containers (yellow/red borders per brand colors)
- [ ] Verify SVG icon rendering quality at 64x64 upscale (Azure icons are 18x18 native)
- [ ] Add Azure/GCP provider logos to cloud boundary labels
- [ ] Visual comparison with official reference diagrams and iterate
