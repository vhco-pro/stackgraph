// Package output renders the internal graph to JSON, DOT, and SVG.
package output

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/michielvha/logger"
	sgraph "github.com/vhco-pro/stackgraph/pkg/graph"
	"github.com/vhco-pro/stackgraph/pkg/icons"
)

// NodePosition holds a node's computed layout position (center-based).
type NodePosition struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

const (
	iconSize      = 64
	nodePadding   = 16
	labelHeight   = 20
	nodeWidth     = iconSize + nodePadding*2                   // 96
	nodeHeight    = iconSize + labelHeight + nodePadding*2 + 8 // ~124
	boxNodeWidth  = 140
	boxNodeHeight = 52
)

// nodeSize returns the layout dimensions for a node.
func nodeSize(n *sgraph.Node) (w, h int) {
	if n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0 {
		// Groups get minimum size — the layout engine expands them to fit children
		return 200, 100
	}
	if n.Icon != "" {
		return nodeWidth, nodeHeight
	}
	// Box nodes are sized to their longest label line so names never truncate.
	svc, name := boxNodeLabelLines(n)
	longest := len([]rune(svc))
	if l := len([]rune(name)); l > longest {
		longest = l
	}
	w = longest*7 + 24
	if w < boxNodeWidth {
		w = boxNodeWidth
	}
	return w, boxNodeHeight
}

// boxNodeLabelLines returns the two label lines for an icon-less box node:
// the service (or resource type) line and the resource name line.
func boxNodeLabelLines(n *sgraph.Node) (service, name string) {
	service = n.Service
	if service == "" {
		service = n.ResourceType
	}
	name = n.Label
	if n.Count > 1 {
		name += fmt.Sprintf(" (x%d)", n.Count)
	}
	return service, name
}

// resolvedStyle returns the container style for a group node: the mapping's
// declared style when present, generic defaults otherwise. Role is clamped to
// the CSS class families the stylesheet defines.
func resolvedStyle(n *sgraph.Node) sgraph.NodeStyle {
	s := sgraph.NodeStyle{
		Fill:   "#F8F9FA",
		Stroke: "#ADB5BD",
		Label:  "#495057",
		Role:   "default",
		Dashed: true,
	}
	if n.Style != nil {
		if n.Style.Fill != "" {
			s.Fill = n.Style.Fill
		}
		if n.Style.Stroke != "" {
			s.Stroke = n.Style.Stroke
		}
		if n.Style.Label != "" {
			s.Label = n.Style.Label
		}
		if n.Style.Role != "" {
			s.Role = n.Style.Role
		}
		s.Dashed = n.Style.Dashed
	}
	switch s.Role {
	case "cloud", "vpc", "subnet", "sg", "default":
	default:
		s.Role = "default"
	}
	return s
}

// actionAccent returns the plan-action accent color for a node, or "" for none.
func actionAccent(a sgraph.Action) string {
	switch a {
	case sgraph.ActionCreate:
		return "#43A047"
	case sgraph.ActionDelete:
		return "#E53935"
	case sgraph.ActionUpdate, sgraph.ActionReplace:
		return "#FB8C00"
	default:
		return ""
	}
}

// containerLabel joins service and name, skipping the join when they are equal
// (synthetic boundary nodes like "AWS Cloud" carry the same text in both).
func containerLabel(n *sgraph.Node) string {
	if n.Service != "" && n.Service != n.Label {
		return n.Service + " — " + n.Label
	}
	if n.Service != "" {
		return n.Service
	}
	return n.Label
}

// renderContainerRect and renderContainerLabel are separate layers so edges can
// render between container backgrounds and their labels.
func renderContainerRect(b *strings.Builder, n *sgraph.Node, pos NodePosition) {
	x := pos.X - pos.Width/2
	y := pos.Y - pos.Height/2

	s := resolvedStyle(n)
	dashAttr := ""
	if s.Dashed {
		dashAttr = ` stroke-dasharray="6,3"`
	}

	fmt.Fprintf(b, `<rect class="sg-container-%s" data-id="%s" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8" fill="%s" stroke="%s" stroke-width="2"%s/>`,
		s.Role, escapeXMLStr(n.ID), x, y, pos.Width, pos.Height, s.Fill, s.Stroke, dashAttr)
	b.WriteString("\n")
}

func renderContainerLabel(b *strings.Builder, n *sgraph.Node, pos NodePosition) {
	x := pos.X - pos.Width/2
	y := pos.Y - pos.Height/2

	label := containerLabel(n)
	s := resolvedStyle(n)

	labelX := x + 12
	if n.Icon != "" {
		iconData := embedIconBase64(n.Icon)
		if iconData != "" {
			fmt.Fprintf(b, `<image href="%s" x="%.0f" y="%.0f" width="24" height="24"/>`, iconData, x+8, y+4)
			b.WriteString("\n")
			labelX = x + 38
		}
	}

	fmt.Fprintf(b, `<text class="sg-label-%s" x="%.0f" y="%.0f" font-size="13" font-weight="600" fill="%s">%s</text>`,
		s.Role, labelX, y+22, s.Label, escapeXMLStr(label))
	b.WriteString("\n")
}

// renderNode draws a leaf node: icon with label below when an icon is mapped,
// a sized box otherwise. Plan actions render as a colored accent ring/border.
func renderNode(b *strings.Builder, n *sgraph.Node, pos NodePosition) {
	cx := pos.X
	cy := pos.Y
	x := cx - pos.Width/2
	y := cy - pos.Height/2

	fmt.Fprintf(b, `<g class="sg-node" data-id="%s">`, escapeXMLStr(n.ID))
	b.WriteString("\n")
	defer func() {
		b.WriteString("</g>\n")
	}()

	iconData := ""
	if n.Icon != "" {
		iconData = embedIconBase64(n.Icon)
	}
	accent := actionAccent(n.Action)

	if iconData != "" {
		// Plan-action accent: rounded ring behind the icon
		if accent != "" {
			fmt.Fprintf(b, `<rect x="%.0f" y="%.0f" width="%d" height="%d" rx="10" fill="none" stroke="%s" stroke-width="3"/>`,
				cx-float64(iconSize)/2-6, y+float64(nodePadding)-6, iconSize+12, iconSize+12, accent)
			b.WriteString("\n")
		}

		iconX := cx - float64(iconSize)/2
		iconY := y + float64(nodePadding)
		fmt.Fprintf(b, `<image href="%s" x="%.0f" y="%.0f" width="%d" height="%d"/>`,
			iconData, iconX, iconY, iconSize, iconSize)
		b.WriteString("\n")

		// Label below icon — same two lines as box nodes, one composition rule
		labelY := iconY + float64(iconSize) + 16
		svc, name := boxNodeLabelLines(n)
		lines := []string{name}
		if svc != n.ResourceType { // suppress the raw type line for unmapped nodes
			lines = []string{svc, name}
		}
		for i, line := range lines {
			fmt.Fprintf(b, `<text class="sg-text" x="%.0f" y="%.0f" text-anchor="middle" font-size="11" fill="#2D3436">%s</text>`,
				cx, labelY+float64(i)*14, escapeXMLStr(line))
			b.WriteString("\n")
		}
		return
	}

	// Generic box node. Plan-action colors go in an inline style attribute:
	// the .sg-node-box class rules would override plain presentation
	// attributes (CSS class > presentation attribute in SVG), but inline
	// style outranks the class in both light and dark mode.
	styleAttr := ""
	if accent != "" {
		fill := "#FFF8E1"
		switch n.Action {
		case sgraph.ActionCreate:
			fill = "#E8F5E9"
		case sgraph.ActionDelete:
			fill = "#FFEBEE"
		}
		styleAttr = fmt.Sprintf(` style="fill:%s;stroke:%s;stroke-width:2"`, fill, accent)
	}

	fmt.Fprintf(b, `<rect class="sg-node-box" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="6" fill="#FFFFFF" stroke="#DEE2E6" stroke-width="1.5"%s/>`,
		x, y, pos.Width, pos.Height, styleAttr)
	b.WriteString("\n")

	svc, name := boxNodeLabelLines(n)
	fmt.Fprintf(b, `<text class="sg-text" x="%.0f" y="%.0f" text-anchor="middle" font-size="11" font-weight="600" fill="#2D3436">%s</text>`,
		cx, cy-3, escapeXMLStr(svc))
	b.WriteString("\n")
	fmt.Fprintf(b, `<text class="sg-text-sm" x="%.0f" y="%.0f" text-anchor="middle" font-size="10" fill="#2D3436">%s</text>`,
		cx, cy+11, escapeXMLStr(name))
	b.WriteString("\n")
}

// iconDataURICache memoizes base64 data URIs per icon path — embedded icons
// are immutable, and a large diagram re-embeds the same EC2 icon many times.
var iconDataURICache sync.Map // iconPath -> data URI string ("" = missing)

func embedIconBase64(iconPath string) string {
	if cached, ok := iconDataURICache.Load(iconPath); ok {
		return cached.(string)
	}

	uri := ""
	if data, err := icons.GetIconBytes(iconPath); err != nil {
		logger.Debugf("icon not found: %s", iconPath)
	} else {
		mime := "image/png"
		if strings.ToLower(filepath.Ext(iconPath)) == ".svg" {
			mime = "image/svg+xml"
		}
		uri = fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	}
	iconDataURICache.Store(iconPath, uri)
	return uri
}

func escapeXMLStr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
