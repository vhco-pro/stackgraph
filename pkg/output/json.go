// Package output provides renderers for converting graphs to various output formats.
package output

import (
	"github.com/michielvha/stackgraph/pkg/graph"
)

// RenderJSON serializes the graph to its JSON representation.
func RenderJSON(g *graph.Graph) ([]byte, error) {
	return g.ToJSON()
}
