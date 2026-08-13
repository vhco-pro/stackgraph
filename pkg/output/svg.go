// Package output renders the internal graph to JSON, DOT, and SVG.
package output

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
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

// Shared palette — single source for the SVG stylesheet and the HTML viewer
// chrome, so a dark-mode retune can't leave the two out of sync.
const (
	paletteLightBG     = "#FFFFFF"
	paletteLightText   = "#2D3436"
	paletteLightBorder = "#DEE2E6"
	paletteLightEdge   = "#7B8894"

	paletteDarkBG     = "#1a1a2e"
	paletteDarkText   = "#E0E0E0"
	paletteDarkTextSm = "#B0B0B0"
	paletteDarkBorder = "#4A5568"
	paletteDarkEdge   = "#9CA3AF"
	paletteDarkBox    = "#2a2a3e"
)

// roleDarkDefault is the built-in dark palette per style role, used when a
// mapping doesn't declare dark_* colors of its own.
var roleDarkDefault = map[string]sgraph.NodeStyle{
	"cloud":   {Fill: "#1e1e32", Stroke: "#4A90D9", Label: "#4A90D9"},
	"vpc":     {Fill: "#1e1530", Stroke: "#A855F7", Label: "#C084FC"},
	"subnet":  {Fill: "#1a2e1a", Stroke: "#8BC34A", Label: "#8BC34A"},
	"sg":      {Fill: "#2e1a1a", Stroke: "#EF5350", Label: "#EF5350"},
	"default": {Fill: "#1e1e2e", Stroke: "#6B7280", Label: "#9CA3AF"},
}

// sanitizeRole restricts a role to CSS-class-safe characters. Unknown roles
// are fine (their dark palette falls back to default); unsafe ones are not.
func sanitizeRole(role string) string {
	if role == "" {
		return "default"
	}
	role = strings.ToLower(role)
	for _, r := range role {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return "default"
		}
	}
	return role
}

// roleDarkCSS generates the dark-mode CSS rules for every container role the
// graph actually uses. Declared dark_* colors win over the built-in palette;
// roles the built-in palette doesn't know inherit the default dark colors.
func roleDarkCSS(g *sgraph.Graph) string {
	type darkStyle struct{ fill, stroke, label string }
	roles := make(map[string]darkStyle)

	for _, n := range g.Nodes {
		if n.Type != sgraph.NodeTypeGroup && len(n.Children) == 0 {
			continue
		}
		s := resolvedStyle(n)
		if _, done := roles[s.Role]; done {
			continue
		}
		base, ok := roleDarkDefault[s.Role]
		if !ok {
			base = roleDarkDefault["default"]
		}
		d := darkStyle{fill: base.Fill, stroke: base.Stroke, label: base.Label}
		if n.Style != nil {
			if n.Style.DarkFill != "" {
				d.fill = n.Style.DarkFill
			}
			if n.Style.DarkStroke != "" {
				d.stroke = n.Style.DarkStroke
			}
			if n.Style.DarkLabel != "" {
				d.label = n.Style.DarkLabel
			}
		}
		roles[s.Role] = d
	}

	names := make([]string, 0, len(roles))
	for role := range roles {
		names = append(names, role)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, role := range names {
		d := roles[role]
		fmt.Fprintf(&b, "    .sg-container-%s { fill: %s; stroke: %s; }\n", role, d.fill, d.stroke)
		fmt.Fprintf(&b, "    .sg-label-%s { fill: %s; }\n", role, d.label)
	}
	return b.String()
}

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
	s.Role = sanitizeRole(s.Role)
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

func renderContainerLabel(b *strings.Builder, n *sgraph.Node, pos NodePosition, iconRefs map[string]string) {
	x := pos.X - pos.Width/2
	y := pos.Y - pos.Height/2

	label := containerLabel(n)
	s := resolvedStyle(n)

	labelX := x + 12
	if n.Icon != "" && iconUse(b, iconRefs, n.Icon, 24, x+8, y+4) {
		labelX = x + 38
	}

	fmt.Fprintf(b, `<text class="sg-label-%s" x="%.0f" y="%.0f" font-size="13" font-weight="600" fill="%s">%s</text>`,
		s.Role, labelX, y+22, s.Label, escapeXMLStr(label))
	b.WriteString("\n")
}

// renderNode draws a leaf node: icon with label below when an icon is mapped,
// a sized box otherwise. Plan actions render as a colored accent ring/border.
func renderNode(b *strings.Builder, n *sgraph.Node, pos NodePosition, iconRefs map[string]string) {
	cx := pos.X
	cy := pos.Y
	x := cx - pos.Width/2
	y := cy - pos.Height/2

	fmt.Fprintf(b, `<g class="sg-node" data-id="%s">`, escapeXMLStr(n.ID))
	b.WriteString("\n")
	defer func() {
		b.WriteString("</g>\n")
	}()

	accent := actionAccent(n.Action)

	_, hasIcon := iconRefs[fmt.Sprintf("%s@%d", n.Icon, iconSize)]
	hasIcon = hasIcon && n.Icon != ""

	if hasIcon {
		// Plan-action accent: rounded ring behind the icon
		if accent != "" {
			fmt.Fprintf(b, `<rect x="%.0f" y="%.0f" width="%d" height="%d" rx="10" fill="none" stroke="%s" stroke-width="3"/>`,
				cx-float64(iconSize)/2-6, y+float64(nodePadding)-6, iconSize+12, iconSize+12, accent)
			b.WriteString("\n")
		}
		iconUse(b, iconRefs, n.Icon, iconSize, cx-float64(iconSize)/2, y+float64(nodePadding))

		// Label below icon — same two lines as box nodes, one composition rule
		labelY := y + float64(nodePadding) + float64(iconSize) + 16
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

// buildIconDefs writes a <defs> block containing the arrowhead marker and one
// shared <image> element per distinct icon+size used by the graph, and returns
// the reference map (iconPath + "@" + size → element id). Nodes then draw
// icons with <use>, so each icon's data URI appears once per document instead
// of once per node.
func buildIconDefs(b *strings.Builder, g *sgraph.Graph) map[string]string {
	fmt.Fprintf(b, `<defs><marker id="arrowhead" markerWidth="10" markerHeight="7" refX="10" refY="3.5" orient="auto"><polygon class="sg-arrow" points="0 0, 10 3.5, 0 7" fill="%s"/></marker>`, paletteLightEdge)
	b.WriteString("\n")

	refs := make(map[string]string)
	add := func(iconPath string, size int) {
		key := fmt.Sprintf("%s@%d", iconPath, size)
		if _, ok := refs[key]; ok {
			return
		}
		data := embedIconBase64(iconPath)
		if data == "" {
			return
		}
		id := fmt.Sprintf("sgi%d", len(refs))
		refs[key] = id
		fmt.Fprintf(b, `<image id="%s" href="%s" width="%d" height="%d"/>`, id, data, size, size)
		b.WriteString("\n")
	}

	for _, n := range g.Nodes {
		if n.Icon == "" {
			continue
		}
		if n.Type == sgraph.NodeTypeGroup || len(n.Children) > 0 {
			add(n.Icon, 24)
		} else {
			add(n.Icon, iconSize)
		}
	}

	b.WriteString("</defs>\n")
	return refs
}

// iconUse writes a <use> referencing a shared icon def, or nothing when the
// icon isn't in the defs (missing file). Returns whether it drew.
func iconUse(b *strings.Builder, refs map[string]string, iconPath string, size int, x, y float64) bool {
	id, ok := refs[fmt.Sprintf("%s@%d", iconPath, size)]
	if !ok {
		return false
	}
	fmt.Fprintf(b, `<use href="#%s" x="%.0f" y="%.0f"/>`, id, x, y)
	b.WriteString("\n")
	return true
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
