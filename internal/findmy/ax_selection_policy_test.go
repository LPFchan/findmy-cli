package findmy

import (
	"fmt"
	"testing"
)

type selectionPolicyNode struct {
	path     string
	parent   string
	actions  []string
	disabled bool
	semantic bool
}

func resolveSelectionPolicy(nodes []selectionPolicyNode, semanticPath string, exactTextPaths []string) (string, error) {
	byPath := make(map[string]selectionPolicyNode, len(nodes))
	for _, node := range nodes {
		byPath[node.path] = node
	}
	actionable := func(node selectionPolicyNode) bool {
		if node.disabled {
			return false
		}
		for _, action := range node.actions {
			if action == "AXPress" || action == "AXPick" {
				return true
			}
		}
		return false
	}
	isDescendant := func(path string) bool {
		current := byPath[path].parent
		for current != "" {
			if current == semanticPath {
				return true
			}
			current = byPath[current].parent
		}
		return false
	}
	row, rowExists := byPath[semanticPath]
	if !rowExists || !row.semantic {
		return "", fmt.Errorf("matched path is not a semantic Accessibility row")
	}

	var exactActions []string
	for _, path := range exactTextPaths {
		if node, ok := byPath[path]; ok && isDescendant(path) && actionable(node) {
			exactActions = append(exactActions, path)
		}
	}
	if len(exactActions) == 1 {
		return exactActions[0], nil
	}
	if len(exactActions) > 1 {
		return "", fmt.Errorf("exact target text exposes multiple actionable elements")
	}

	var descendantActions []string
	for _, node := range nodes {
		if node.path != semanticPath && isDescendant(node.path) && actionable(node) {
			descendantActions = append(descendantActions, node.path)
		}
	}
	if len(descendantActions) == 1 {
		return descendantActions[0], nil
	}
	if len(descendantActions) > 1 {
		return "", fmt.Errorf("semantic row exposes multiple actionable descendants")
	}
	if actionable(row) {
		return semanticPath, nil
	}
	return "", fmt.Errorf("semantic row exposes no actionable selection element")
}

func TestSelectionPolicyPrecedenceAndBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		nodes      []selectionPolicyNode
		exactPaths []string
		want       string
		wantError  bool
	}{
		{
			name: "exact target text wins over another descendant",
			nodes: []selectionPolicyNode{
				{path: "row", semantic: true},
				{path: "row.target", parent: "row", actions: []string{"AXPress"}},
				{path: "row.other", parent: "row", actions: []string{"AXPress"}},
			},
			exactPaths: []string{"row.target"},
			want:       "row.target",
		},
		{
			name: "unique actionable descendant",
			nodes: []selectionPolicyNode{
				{path: "row", semantic: true},
				{path: "row.target", parent: "row"},
				{path: "row.control", parent: "row", actions: []string{"AXPick"}},
			},
			exactPaths: []string{"row.target"},
			want:       "row.control",
		},
		{
			name: "actionable semantic row fallback",
			nodes: []selectionPolicyNode{
				{path: "row", actions: []string{"AXPress"}, semantic: true},
				{path: "row.target", parent: "row"},
			},
			exactPaths: []string{"row.target"},
			want:       "row",
		},
		{
			name: "outside candidate is rejected",
			nodes: []selectionPolicyNode{
				{path: "row", semantic: true},
				{path: "row.target", parent: "row"},
				{path: "other.control", parent: "other", actions: []string{"AXPress"}},
			},
			exactPaths: []string{"row.target"},
			wantError:  true,
		},
		{
			name: "outside exact target candidate is rejected",
			nodes: []selectionPolicyNode{
				{path: "row", semantic: true},
				{path: "other.target", parent: "other", actions: []string{"AXPress"}},
			},
			exactPaths: []string{"other.target"},
			wantError:  true,
		},
		{
			name: "multiple exact target actions fail closed",
			nodes: []selectionPolicyNode{
				{path: "row", semantic: true},
				{path: "row.target1", parent: "row", actions: []string{"AXPress"}},
				{path: "row.target2", parent: "row", actions: []string{"AXPress"}},
			},
			exactPaths: []string{"row.target1", "row.target2"},
			wantError:  true,
		},
		{
			name: "multiple descendant actions fail closed",
			nodes: []selectionPolicyNode{
				{path: "row", actions: []string{"AXPress"}, semantic: true},
				{path: "row.target", parent: "row"},
				{path: "row.first", parent: "row", actions: []string{"AXPress"}},
				{path: "row.second", parent: "row", actions: []string{"AXPick"}},
			},
			exactPaths: []string{"row.target"},
			wantError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSelectionPolicy(test.nodes, "row", test.exactPaths)
			if test.wantError {
				if err == nil {
					t.Fatalf("resolveSelectionPolicy() = %q, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("resolveSelectionPolicy() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
