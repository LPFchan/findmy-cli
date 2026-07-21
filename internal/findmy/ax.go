package findmy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type AXFrame struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type AXNode struct {
	Path        string   `json:"path"`
	ParentPath  *string  `json:"parentPath,omitempty"`
	Role        string   `json:"role"`
	Subrole     string   `json:"subrole"`
	Title       string   `json:"title"`
	Value       string   `json:"value"`
	Description string   `json:"description"`
	Identifier  string   `json:"identifier"`
	Selected    *bool    `json:"selected,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Frame       *AXFrame `json:"frame,omitempty"`
	Actions     []string `json:"actions"`
}

type AXTree struct {
	PID   int      `json:"pid"`
	Nodes []AXNode `json:"nodes"`
}

type AXRecord struct {
	Name      string
	Location  string
	Staleness string
	Distance  string
	Battery   string
	Path      string
}

type PlaySoundResult struct {
	OK        bool   `json:"ok"`
	Action    string `json:"action"`
	Device    string `json:"device"`
	Confirmed bool   `json:"confirmed"`
	DryRun    bool   `json:"dry_run"`
	Error     string `json:"error,omitempty"`
}

func ReadAXTree() (AXTree, error) {
	if err := requireAccessibility(); err != nil {
		return AXTree{}, err
	}
	out, err := runHelper("tree")
	if err != nil {
		return AXTree{}, fmt.Errorf("helper AX tree: %w", err)
	}
	var tree AXTree
	if err := json.Unmarshal(out, &tree); err != nil {
		return AXTree{}, fmt.Errorf("decode AX tree: %w", err)
	}
	if len(tree.Nodes) == 0 {
		return AXTree{}, fmt.Errorf("Find My returned an empty Accessibility tree")
	}
	return tree, nil
}

func prepareAXTab(tab string) (AXTree, error) {
	if err := requireAccessibility(); err != nil {
		return AXTree{}, err
	}
	if err := Activate(); err != nil {
		return AXTree{}, fmt.Errorf("activate Find My: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := SwitchTab(tab); err != nil {
		return AXTree{}, fmt.Errorf("select %s tab: %w", tab, err)
	}
	time.Sleep(700 * time.Millisecond)
	return ReadAXTree()
}

func ReadPeopleAX() ([]Person, error) {
	tree, err := prepareAXTab(GetAppStrings().PeopleTab)
	if err != nil {
		return nil, err
	}
	rows, err := ParseAXRows(tree)
	if err != nil {
		return nil, err
	}
	out := make([]Person, 0, len(rows))
	for _, row := range rows {
		out = append(out, Person{
			Name: row.Name, Location: row.Location,
			Staleness: row.Staleness, Distance: row.Distance,
		})
	}
	return out, nil
}

func ReadDevicesAX() ([]Device, error) {
	tree, err := prepareAXTab(GetAppStrings().DevicesTab)
	if err != nil {
		return nil, err
	}
	rows, err := ParseAXRows(tree)
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(rows))
	for _, row := range rows {
		out = append(out, Device{
			Name: row.Name, Location: row.Location,
			Staleness: row.Staleness, Distance: row.Distance,
			Battery: row.Battery,
		})
	}
	return out, nil
}

func ReadItemsAX() ([]Item, error) {
	tree, err := prepareAXTab(GetAppStrings().ItemsTab)
	if err != nil {
		return nil, err
	}
	rows, err := ParseAXRows(tree)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, Item{
			Name: row.Name, Location: row.Location,
			Staleness: row.Staleness, Distance: row.Distance,
			Battery: row.Battery,
		})
	}
	return out, nil
}

type axText struct {
	text string
	node AXNode
}

func ParseAXRows(tree AXTree) ([]AXRecord, error) {
	window := mainAXWindow(tree.Nodes)
	if window == nil || window.Frame == nil {
		return nil, fmt.Errorf("Find My Accessibility tree has no main window")
	}
	sidebarRight := window.Frame.X + minFloat(480, window.Frame.Width*0.48)
	skip := GetAppStrings().SkipWords()

	var texts []axText
	for _, node := range tree.Nodes {
		if node.Role != "AXStaticText" || node.Frame == nil || node.Frame.Width <= 0 || node.Frame.Height <= 0 {
			continue
		}
		if node.Frame.X >= sidebarRight || node.Frame.Y < window.Frame.Y+45 {
			continue
		}
		text := firstAXText(node)
		if text == "" || skip[text] {
			continue
		}
		texts = append(texts, axText{text: text, node: node})
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("Find My sidebar has no accessible text rows; make sure the sidebar is visible")
	}

	groups := groupAXTexts(texts, tree.Nodes)
	rows := make([]AXRecord, 0, len(groups))
	for _, group := range groups {
		if row, ok := parseAXGroup(group); ok {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Find My sidebar Accessibility text could not be grouped into records")
	}
	return rows, nil
}

func mainAXWindow(nodes []AXNode) *AXNode {
	var best *AXNode
	for i := range nodes {
		if nodes[i].Role != "AXWindow" || nodes[i].Frame == nil {
			continue
		}
		if best == nil || nodes[i].Frame.Width*nodes[i].Frame.Height > best.Frame.Width*best.Frame.Height {
			best = &nodes[i]
		}
	}
	return best
}

func firstAXText(node AXNode) string {
	for _, candidate := range []string{node.Value, node.Title, node.Description} {
		if text := strings.Join(strings.Fields(candidate), " "); text != "" {
			return text
		}
	}
	return ""
}

func groupAXTexts(texts []axText, nodes []AXNode) [][]axText {
	byPath := make(map[string]AXNode, len(nodes))
	for _, node := range nodes {
		byPath[node.Path] = node
	}
	groups := map[string][]axText{}
	for _, text := range texts {
		key := nearestRowPath(text.node.Path, byPath)
		if key == "" {
			centerY := text.node.Frame.Y + text.node.Frame.Height/2
			key = fmt.Sprintf("y:%d", int(centerY/34))
		}
		groups[key] = append(groups[key], text)
	}
	out := make([][]axText, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].node.Frame.Y == group[j].node.Frame.Y {
				return group[i].node.Frame.X < group[j].node.Frame.X
			}
			return group[i].node.Frame.Y < group[j].node.Frame.Y
		})
		out = append(out, group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i][0].node.Frame.Y < out[j][0].node.Frame.Y
	})
	return out
}

func nearestRowPath(path string, nodes map[string]AXNode) string {
	current := path
	for current != "" {
		node, ok := nodes[current]
		if !ok || node.ParentPath == nil {
			break
		}
		parent := nodes[*node.ParentPath]
		switch parent.Role {
		case "AXRow", "AXCell", "AXListItem":
			return parent.Path
		case "AXGroup":
			if parent.Selected != nil || containsAXAction(parent.Actions) {
				return parent.Path
			}
		}
		current = parent.Path
	}
	return ""
}

func containsAXAction(actions []string) bool {
	for _, action := range actions {
		if action == "AXPress" || action == "AXPick" {
			return true
		}
	}
	return false
}

func parseAXGroup(group []axText) (AXRecord, bool) {
	seen := map[string]bool{}
	var values []string
	for _, text := range group {
		value := strings.TrimSpace(text.text)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	if len(values) == 0 {
		return AXRecord{}, false
	}
	row := AXRecord{Name: values[0], Path: group[0].node.Path}
	if path := nearestActionPath(group[0].node.Path, group); path != "" {
		row.Path = path
	}
	for _, value := range values[1:] {
		switch {
		case isDistance(value):
			row.Distance = value
		case isBattery(value):
			row.Battery = value
		case row.Location == "":
			row.Location, row.Staleness = splitLocationStaleness(value)
		case row.Staleness == "":
			row.Staleness = value
		}
	}
	return row, row.Name != ""
}

func nearestActionPath(fallback string, group []axText) string {
	for _, text := range group {
		for _, action := range text.node.Actions {
			if action == "AXPress" || action == "AXPick" {
				return text.node.Path
			}
		}
	}
	return fallback
}

func ResolveDevice(devices []Device, query string) (*Device, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("device name is required")
	}
	var exact, partial []*Device
	for i := range devices {
		name := strings.TrimSpace(devices[i].Name)
		if strings.EqualFold(name, query) {
			exact = append(exact, &devices[i])
		} else if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
			partial = append(partial, &devices[i])
		}
	}
	matches := partial
	if len(exact) > 0 {
		matches = exact
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no device matching %q", query)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			names = append(names, match.Name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("device match %q is ambiguous: %s", query, strings.Join(names, ", "))
	}
}

func LookupDevice(devices []Device, query string) (*Device, error) {
	query = strings.TrimSpace(query)
	for i := range devices {
		if strings.EqualFold(strings.TrimSpace(devices[i].Name), query) {
			return &devices[i], nil
		}
	}
	for i := range devices {
		if strings.Contains(strings.ToLower(devices[i].Name), strings.ToLower(query)) {
			return &devices[i], nil
		}
	}
	return nil, fmt.Errorf("no device matching %q in sidebar", query)
}

func ReadAXDetail(tabLabel, target string) (precise, city, region, postal string, err error) {
	if strings.TrimSpace(target) == "" {
		return "", "", "", "", fmt.Errorf("record name is required")
	}
	if _, err := runHelper("select-record", "--target", target, "--tab-label", tabLabel); err != nil {
		return "", "", "", "", err
	}
	time.Sleep(600 * time.Millisecond)
	tree, err := ReadAXTree()
	if err != nil {
		return "", "", "", "", err
	}
	window := mainAXWindow(tree.Nodes)
	if window == nil || window.Frame == nil {
		return "", "", "", "", fmt.Errorf("Find My Accessibility tree has no main window")
	}
	sidebarRight := int(window.Frame.X + minFloat(480, window.Frame.Width*0.48))
	lines := make([]TextLine, 0)
	for _, node := range tree.Nodes {
		if node.Role != "AXStaticText" || node.Frame == nil {
			continue
		}
		text := firstAXText(node)
		if text == "" {
			continue
		}
		lines = append(lines, TextLine{
			Text: text, X: int(node.Frame.X), Y: int(node.Frame.Y),
			Width: int(node.Frame.Width), Height: int(node.Frame.Height),
		})
	}
	precise, city, region, postal = ExtractDetailPaneAddress(lines, sidebarRight)
	return precise, city, region, postal, nil
}

func ReadDeviceDetail(device Device) (precise, city, region, postal string, err error) {
	return ReadAXDetail(GetAppStrings().DevicesTab, device.Name)
}

func ReadPersonDetail(person Person) (precise, city, region, postal string, err error) {
	return ReadAXDetail(GetAppStrings().PeopleTab, person.Name)
}

func PlaySound(device Device) error {
	if strings.TrimSpace(device.Name) == "" {
		return fmt.Errorf("device name is required")
	}
	args := []string{
		"play-sound", "--target", device.Name,
		"--devices-label", GetAppStrings().DevicesTab,
		"--confirmed",
	}
	for _, label := range GetAppStrings().PlaySoundLabels() {
		args = append(args, "--label", label)
	}
	if _, err := runHelper(args...); err != nil {
		return fmt.Errorf("play sound for %q: %w", device.Name, err)
	}
	return nil
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
