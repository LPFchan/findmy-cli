package findmy

import (
	"strings"
	"sync"
	"testing"
)

func boolPtr(value bool) *bool { return &value }

func testAXTree(rows [][]string) AXTree {
	windowParent := "0"
	nodes := []AXNode{
		{Path: "0", Role: "AXWindow", Frame: &AXFrame{X: 100, Y: 100, Width: 1000, Height: 700}},
	}
	for rowIndex, values := range rows {
		rowPath := "0." + string(rune('1'+rowIndex))
		nodes = append(nodes, AXNode{
			Path: rowPath, ParentPath: &windowParent, Role: "AXRow",
			Selected: boolPtr(rowIndex == 0),
			Frame:    &AXFrame{X: 120, Y: float64(200 + rowIndex*70), Width: 330, Height: 60},
			Actions:  []string{"AXPress"},
		})
		for valueIndex, value := range values {
			parent := rowPath
			nodes = append(nodes, AXNode{
				Path: rowPath + "." + string(rune('0'+valueIndex)), ParentPath: &parent,
				Role: "AXStaticText", Value: value,
				Frame: &AXFrame{X: 210, Y: float64(205 + rowIndex*70 + valueIndex*18), Width: 210, Height: 16},
			})
		}
	}
	return AXTree{PID: 42, Nodes: nodes}
}

func TestParseAXRowsGroupsSemanticRows(t *testing.T) {
	tree := testAXTree([][]string{
		{"Phone", "Seoul • Now", "3 km", "82%"},
		{"Watch", "No location found"},
	})
	rows, err := ParseAXRows(tree)
	if err != nil {
		t.Fatalf("ParseAXRows returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if got := rows[0]; got.Name != "Phone" || got.Location != "Seoul" || got.Staleness != "Now" || got.Distance != "3 km" || got.Battery != "82%" {
		t.Fatalf("first row = %#v", got)
	}
	if rows[0].Path == "" {
		t.Fatal("first row has no actionable AX path")
	}
}

func TestParseAXRowsKoreanTextIsDataNotControl(t *testing.T) {
	appStringsOnce = sync.Once{}
	appStringsOnce.Do(func() { appStrings = lookupStrings("ko") })
	t.Cleanup(func() {
		appStrings = nil
		appStringsOnce = sync.Once{}
	})

	tree := testAXTree([][]string{
		{"기기", "검색"},
		{"내 아이폰", "서울특별시 • 지금", "2 km"},
	})
	rows, err := ParseAXRows(tree)
	if err != nil {
		t.Fatalf("ParseAXRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "내 아이폰" || rows[0].Location != "서울특별시" || rows[0].Staleness != "지금" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestResolveDeviceUniqueAndAmbiguous(t *testing.T) {
	devices := []Device{{Name: "Omar's iPhone"}, {Name: "Omar's iPad"}, {Name: "Watch"}}
	match, err := ResolveDevice(devices, "watch")
	if err != nil || match.Name != "Watch" {
		t.Fatalf("ResolveDevice exact = %#v, %v", match, err)
	}
	if _, err := ResolveDevice(devices, "Omar"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ResolveDevice ambiguous error = %v", err)
	}
	if _, err := ResolveDevice(devices, "missing"); err == nil || !strings.Contains(err.Error(), "no device") {
		t.Fatalf("ResolveDevice missing error = %v", err)
	}
	partial, err := ResolveDevice(devices, "iPad")
	if err != nil || partial.Name != "Omar's iPad" {
		t.Fatalf("ResolveDevice unique partial = %#v, %v", partial, err)
	}
	duplicates := []Device{{Name: "Phone"}, {Name: "phone"}}
	if _, err := ResolveDevice(duplicates, "PHONE"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ResolveDevice duplicate exact error = %v", err)
	}
}
