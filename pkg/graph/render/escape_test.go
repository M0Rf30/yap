package render

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/graph"
)

const evil = `</text><script>alert(1)</script>&"'`

func TestSVGContentEscapesUntrustedText(t *testing.T) {
	graphData := &graph.Data{
		Nodes: map[string]*graph.Node{
			"a": {
				Name: evil, PkgName: evil, Version: evil, Release: evil,
				X: 100, Y: 50, Width: 80, Height: 40,
			},
			"b": {
				Name: "b", PkgName: "b", Version: "1", Release: "1",
				X: 300, Y: 200, Width: 80, Height: 40,
			},
		},
		Edges: []graph.Edge{{From: "a", To: "b", Type: evil}},
	}

	var sb strings.Builder

	addEdges(&sb, graphData, true)
	addNodes(&sb, graphData, true)

	out := sb.String()
	if strings.Contains(out, "<script>") || strings.Contains(out, "</text><") {
		t.Fatalf("unescaped markup in SVG output: %s", out)
	}

	// The fragment must be well-formed XML.
	dec := xml.NewDecoder(strings.NewReader("<root>" + out + "</root>"))
	for {
		if _, err := dec.Token(); err != nil {
			if err.Error() != "EOF" {
				t.Fatalf("SVG fragment is not well-formed: %v", err)
			}

			break
		}
	}
}
