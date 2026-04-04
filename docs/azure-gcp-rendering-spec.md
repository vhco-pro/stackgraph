---
description: "Spec for fixing Azure and GCP diagram rendering to match official architecture diagram standards"
status: proposed
status_description: "Azure/GCP diagrams currently have wrong icons, missing containers, and don't match official visual standards"
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

- [ ] Verify all Azure icon file paths in getIconPath() match actual files in pkg/icons/icons/azure/
- [ ] Verify all GCP icon file paths match actual files in pkg/icons/icons/gcp/
- [ ] Fix any wrong icon path mappings
- [ ] Add Azure VNet/Subnet as group nodes in YAML mappings (if not already)
- [ ] Add GCP VPC/Subnet as group nodes in YAML mappings
- [ ] Add Azure subscription boundary (similar to AWS Cloud boundary)
- [ ] Test with both dagre and ELK layout engines
- [ ] Regenerate Azure and GCP examples
- [ ] Visual comparison with official reference diagrams
