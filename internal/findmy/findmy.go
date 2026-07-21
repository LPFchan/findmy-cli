package findmy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type TextLine struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
}

type Person struct {
	Name           string `json:"name"`
	Location       string `json:"location,omitempty"`
	Staleness      string `json:"staleness,omitempty"`
	Distance       string `json:"distance,omitempty"`
	PreciseAddress string `json:"precise_address,omitempty"`
	City           string `json:"city,omitempty"`
	Region         string `json:"region,omitempty"`
	PostalCode     string `json:"postal_code,omitempty"`
}

type Device struct {
	Name           string `json:"name"`
	Location       string `json:"location,omitempty"`
	Staleness      string `json:"staleness,omitempty"`
	Distance       string `json:"distance,omitempty"`
	Battery        string `json:"battery,omitempty"`
	PreciseAddress string `json:"precise_address,omitempty"`
	City           string `json:"city,omitempty"`
	Region         string `json:"region,omitempty"`
	PostalCode     string `json:"postal_code,omitempty"`
}

type Item struct {
	Name      string `json:"name"`
	Location  string `json:"location,omitempty"`
	Staleness string `json:"staleness,omitempty"`
	Distance  string `json:"distance,omitempty"`
	Battery   string `json:"battery,omitempty"`
}

func helper() string {
	if env := os.Getenv("FINDMY_HELPER"); env != "" {
		return env
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "findmy-helper")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if path, err := exec.LookPath("findmy-helper"); err == nil {
		return path
	}
	return "findmy-helper"
}

func runHelper(args ...string) ([]byte, error) {
	cmd := exec.Command(helper(), args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

type Permissions struct {
	Accessibility bool `json:"accessibility"`
}

// CheckPermissions reports the Accessibility grant used to inspect and act on
// Find My's accessibility tree. Screen Recording is not required.
func CheckPermissions() (Permissions, error) {
	out, err := runHelper("permissions")
	if err != nil {
		return Permissions{}, fmt.Errorf("helper permissions: %w", err)
	}
	var p Permissions
	if err := json.Unmarshal(out, &p); err != nil {
		return Permissions{}, fmt.Errorf("decode permissions: %w", err)
	}
	return p, nil
}

func requireAccessibility() error {
	p, err := CheckPermissions()
	if err != nil {
		return err
	}
	if p.Accessibility {
		return nil
	}
	return fmt.Errorf(
		"missing Accessibility permission for findmy-helper. Grant that helper executable in System Settings → Privacy & Security → Accessibility, then relaunch the calling application",
	)
}

func Activate() error {
	return exec.Command("open", "-b", "com.apple.findmy").Run()
}

func SwitchTab(name string) error {
	_, err := runHelper("select-tab", "--label", name)
	return err
}

// RequireSidebarVisible returns an error when the People/Devices/Items
// segmented control is missing from normalized UI text, which happens when the
// user has hidden the sidebar (View → Hide Sidebar, or the toggle button).
// Without this gate the sidebar parsers see only map content and can yield
// nonsense rows pulled from map labels (place names, road names) rather than
// actual people or devices.
func RequireSidebarVisible(lines []TextLine, sidebarRightPx int, tabName string) error {
	seenPeople := false
	seenOtherTab := false
	for _, l := range lines {
		txt := strings.TrimSpace(l.Text)
		isPeople, isTab := sidebarTabText(txt)
		if !isTab {
			continue
		}
		if l.Y > 220 {
			continue
		}
		if l.X+l.Width/2 >= sidebarRightPx {
			continue
		}
		if isPeople {
			seenPeople = true
		} else {
			seenOtherTab = true
		}
	}
	if seenPeople && seenOtherTab {
		return nil
	}
	return fmt.Errorf("Find My sidebar is not visible. Open the sidebar, select %s, then re-run findmy", tabName)
}

// ParsePeople groups normalized text lines from the People sidebar into records.
// It remains for parser compatibility; live extraction uses ParseAXRows.
// The sidebar layout (in image pixels) has three bands:
//
//	avatar:     x ≈   0–200   (round photo with initials — text noise lives here)
//	text:       x ≈ 240–550   (name and location/staleness)
//	distance:   x ≈ 580–700   ("1,971 mi" right-aligned to row top)
//
// We discard the avatar band entirely (it produces low-confidence fragments
// like "Is" or "rk" from initials and shadows that otherwise get misread as
// person names), then walk the remaining lines top-to-bottom. The sidebar's
// right edge and the y-cutoff for the first row are derived from the observed
// People/Devices/Items tab-pill positions (see detectSidebarRight,
// detectSidebarRowStartY) rather than fixed at scaled-point constants — the
// dynamic bounds handle compact Catalyst layouts where map labels would
// otherwise bleed into the fixed cutoff.
func ParsePeople(lines []TextLine, sidebarRightPx, textColMinPx int) []Person {
	rows := make([]TextLine, 0, len(lines))
	effectiveSidebarRightPx := detectSidebarRight(lines, sidebarRightPx)
	rowStartY := detectSidebarRowStartY(lines, effectiveSidebarRightPx)
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if l.X+l.Width/2 >= effectiveSidebarRightPx {
			continue
		}
		if l.Y < rowStartY {
			continue
		}
		if l.X < textColMinPx {
			continue
		}
		rows = append(rows, l)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Y == rows[j].Y {
			return rows[i].X < rows[j].X
		}
		return rows[i].Y < rows[j].Y
	})
	rows = mergeWrappedContinuations(rows)

	skip := GetAppStrings().SkipWords()

	people := make([]Person, 0)
	var current *Person
	for _, l := range rows {
		txt := strings.TrimSpace(l.Text)
		if skip[txt] {
			continue
		}
		if isDistance(txt) {
			if current != nil {
				current.Distance = txt
			}
			continue
		}
		if current == nil || current.Location != "" {
			people = append(people, Person{Name: txt})
			current = &people[len(people)-1]
			continue
		}
		loc, stale := splitLocationStaleness(txt)
		current.Location = loc
		current.Staleness = stale
	}
	return people
}

// detectSidebarRight returns the narrower of (the observed right edge of
// the People/Devices/Items segmented control + 40px padding) and the
// scaled-point fallback, so that on compact Catalyst layouts the cutoff
// shrinks to exclude map labels that start near x≈350px. Returns the
// fallback unchanged when no tab pill is present.
func detectSidebarRight(lines []TextLine, fallbackRightPx int) int {
	maxTabRight := 0
	for _, l := range lines {
		txt := strings.TrimSpace(l.Text)
		if _, isTab := sidebarTabText(txt); !isTab {
			continue
		}
		if l.Y > 220 {
			continue
		}
		if r := l.X + l.Width; r > maxTabRight {
			maxTabRight = r
		}
	}
	if maxTabRight == 0 {
		return fallbackRightPx
	}
	observed := maxTabRight + 40
	if observed < fallbackRightPx {
		return observed
	}
	return fallbackRightPx
}

// detectSidebarRowStartY returns the y-coordinate (image pixels) below
// which actual sidebar rows begin, computed as the bottom of the tab-pill
// band plus 12px padding. The 120px fallback is effectively unreachable
// because callers gate on RequireSidebarVisible — if there's no tab pill,
// parsing is short-circuited before this is consulted.
func detectSidebarRowStartY(lines []TextLine, sidebarRightPx int) int {
	const fallbackY = 120
	bottom := 0
	for _, l := range lines {
		txt := strings.TrimSpace(l.Text)
		if _, isTab := sidebarTabText(txt); !isTab {
			continue
		}
		if l.X+l.Width/2 >= sidebarRightPx {
			continue
		}
		if l.Y > 220 {
			continue
		}
		if b := l.Y + l.Height; b > bottom {
			bottom = b
		}
	}
	if bottom == 0 {
		return fallbackY
	}
	return bottom + 12
}

func sidebarTabText(txt string) (isPeople, isTab bool) {
	ls := GetAppStrings()
	switch txt {
	case "People", ls.PeopleTab:
		return true, true
	case "Devices", "Items", ls.DevicesTab, ls.ItemsTab:
		return false, true
	default:
		return false, false
	}
}

// mergeWrappedContinuations folds text fragments split across two
// visual rows because of a long "City, ST • 2 min. ago" string. The
// telltale: the previous row contains the " • " separator and the next row
// is within ~35px below it and looks like a relative-time suffix.
func mergeWrappedContinuations(rows []TextLine) []TextLine {
	out := make([]TextLine, 0, len(rows))
	for _, l := range rows {
		if n := len(out); n > 0 {
			prev := &out[n-1]
			gap := l.Y - (prev.Y + prev.Height)
			if gap < 12 && strings.Contains(prev.Text, "•") && looksLikeTimeSuffix(l.Text) {
				prev.Text = prev.Text + " " + strings.TrimSpace(l.Text)
				if l.Y+l.Height > prev.Y+prev.Height {
					prev.Height = (l.Y + l.Height) - prev.Y
				}
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

func looksLikeTimeSuffix(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	for _, pattern := range GetAppStrings().TimeSuffixes {
		if t == pattern || strings.HasSuffix(t, pattern) || strings.Contains(t, pattern) {
			return true
		}
	}
	return false
}

func isDistance(s string) bool {
	s = strings.ToLower(s)
	for _, suffix := range []string{" mi", " km", " ft", " m", " yd"} {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// splitLocationStaleness parses a "<location> • <staleness>" sidebar line into
// its two halves. One wrinkle: when the device is the Mac the user is
// currently running on, FindMy.app puts a "This Mac" badge in the location
// slot instead of an actual place ("This Mac • No location found"). The badge
// is not a location, so when the left half is one of the known "This <device>"
// labels we treat the right half as the location and drop the badge.
func splitLocationStaleness(s string) (location, staleness string) {
	idx := strings.Index(s, "•")
	if idx < 0 {
		return s, ""
	}
	left := strings.TrimSpace(s[:idx])
	right := strings.TrimSpace(s[idx+len("•"):])
	if isThisDeviceLabel(left) {
		return right, ""
	}
	return left, right
}

// isThisDeviceLabel reports whether s is the "This Mac" / "This iPhone" /
// "This iPad" badge FindMy.app shows on the device the user is currently
// signed into. The label is English-only here; other locales translate it
// (e.g. "Ce Mac" in French) but those strings are not yet catalogued.
func isThisDeviceLabel(s string) bool {
	switch s {
	case "This Mac", "This iPhone", "This iPad", "This Apple Watch":
		return true
	}
	return false
}

// ParseDevices groups normalized text lines from the Devices sidebar into records.
// It remains for parser compatibility; live extraction uses ParseAXRows.
// Layout mirrors People (avatar/icon column on left, text band middle, distance
// right) but rows can also carry a battery indicator such as "82%".
// Battery percentages are extracted into the Battery field; everything else
// follows the same row-walk logic as ParsePeople.
func ParseDevices(lines []TextLine, sidebarRightPx, textColMinPx int) []Device {
	rows := make([]TextLine, 0, len(lines))
	effectiveSidebarRightPx := detectSidebarRight(lines, sidebarRightPx)
	rowStartY := detectSidebarRowStartY(lines, effectiveSidebarRightPx)
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if l.X+l.Width/2 >= effectiveSidebarRightPx {
			continue
		}
		if l.Y < rowStartY {
			continue
		}
		if l.X < textColMinPx {
			continue
		}
		rows = append(rows, l)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Y == rows[j].Y {
			return rows[i].X < rows[j].X
		}
		return rows[i].Y < rows[j].Y
	})
	rows = mergeWrappedContinuations(rows)

	skip := GetAppStrings().SkipWords()

	devices := make([]Device, 0)
	var current *Device
	for _, l := range rows {
		txt := strings.TrimSpace(l.Text)
		if skip[txt] {
			continue
		}
		if isDistance(txt) {
			if current != nil {
				current.Distance = txt
			}
			continue
		}
		if isBattery(txt) {
			if current != nil {
				current.Battery = txt
			}
			continue
		}
		if current == nil || current.Location != "" {
			devices = append(devices, Device{Name: txt})
			current = &devices[len(devices)-1]
			continue
		}
		loc, stale := splitLocationStaleness(txt)
		current.Location = loc
		current.Staleness = stale
	}
	return devices
}

// ParseItems groups normalized text lines from the Items sidebar into records.
// It remains for parser compatibility; live extraction uses ParseAXRows.
// The layout mirrors Devices (icon column on left, text band middle, distance
// right) and can also carry a battery indicator such as "82%".
func ParseItems(lines []TextLine, sidebarRightPx, textColMinPx int) []Item {
	rows := make([]TextLine, 0, len(lines))
	effectiveSidebarRightPx := detectSidebarRight(lines, sidebarRightPx)
	rowStartY := detectSidebarRowStartY(lines, effectiveSidebarRightPx)
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if l.X+l.Width/2 >= effectiveSidebarRightPx {
			continue
		}
		if l.Y < rowStartY {
			continue
		}
		if l.X < textColMinPx {
			continue
		}
		rows = append(rows, l)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Y == rows[j].Y {
			return rows[i].X < rows[j].X
		}
		return rows[i].Y < rows[j].Y
	})
	rows = mergeWrappedContinuations(rows)

	skip := GetAppStrings().SkipWords()

	items := make([]Item, 0)
	var current *Item
	for _, l := range rows {
		txt := strings.TrimSpace(l.Text)
		if skip[txt] {
			continue
		}
		if isDistance(txt) {
			if current != nil {
				current.Distance = txt
			}
			continue
		}
		if isBattery(txt) {
			if current != nil {
				current.Battery = txt
			}
			continue
		}
		if current == nil || current.Location != "" {
			items = append(items, Item{Name: txt})
			current = &items[len(items)-1]
			continue
		}
		loc, stale := splitLocationStaleness(txt)
		current.Location = loc
		current.Staleness = stale
	}
	return items
}

// isBattery recognizes FindMy.app battery-indicator text. The Devices tab
// renders a battery glyph followed by a percentage like "82%". A bare "Offline" or "No
// location" is left to fall through and become the device's Status row.
func isBattery(s string) bool {
	t := strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	if !strings.HasSuffix(t, "%") {
		return false
	}
	num := strings.TrimSuffix(t, "%")
	if num == "" {
		return false
	}
	for _, r := range num {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

var cityRegionPostalRE = regexp.MustCompile(`^([A-Za-z .'-]+),\s*([A-Z]{2})\s*(\d{5}(?:-\d{4})?)?$`)

// ExtractDetailPaneAddress filters accessible text to FindMy's right-side detail
// pane and extracts the address rendered below the selected person/device
// header. US addresses are split into city/region/postal when possible;
// otherwise the visible address lines are returned as a single precise address.
func ExtractDetailPaneAddress(lines []TextLine, sidebarRightPx int) (precise, city, region, postal string) {
	rightPane := make([]TextLine, 0, len(lines))
	for _, l := range lines {
		txt := strings.TrimSpace(l.Text)
		if txt == "" {
			continue
		}
		if l.X <= sidebarRightPx+20 {
			continue
		}
		rightPane = append(rightPane, l)
	}
	sort.SliceStable(rightPane, func(i, j int) bool {
		if rightPane[i].Y == rightPane[j].Y {
			return rightPane[i].X < rightPane[j].X
		}
		return rightPane[i].Y < rightPane[j].Y
	})

	addressLines := make([]string, 0, 3)
	seenHeader := false
	for _, l := range rightPane {
		txt := normalizeDetailPaneText(l.Text)
		if txt == "" {
			continue
		}
		if skipDetailPaneText(txt) {
			if len(addressLines) > 0 {
				break
			}
			continue
		}
		if !seenHeader {
			seenHeader = true
			continue
		}
		if len(addressLines) > 0 && !looksLikeAddressLine(txt) {
			break
		}
		addressLines = append(addressLines, txt)
		if _, _, _, ok := parseCityRegionPostal(txt); ok {
			break
		}
		if len(addressLines) >= 4 {
			break
		}
	}

	if len(addressLines) == 0 {
		return "", "", "", ""
	}
	for i, line := range addressLines {
		if c, r, p, ok := parseCityRegionPostal(line); ok {
			if i == 0 {
				return line, c, r, p
			}
			return strings.Join(addressLines[:i], ", "), c, r, p
		}
	}
	return strings.Join(addressLines, ", "), "", "", ""
}

func normalizeDetailPaneText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func parseCityRegionPostal(s string) (city, region, postal string, ok bool) {
	m := cityRegionPostalRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

func looksLikeAddressLine(s string) bool {
	if _, _, _, ok := parseCityRegionPostal(s); ok {
		return true
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	t := strings.ToLower(s)
	for _, word := range []string{"street", "st", "avenue", "ave", "road", "rd", "drive", "dr", "lane", "ln", "way", "place", "pl", "court", "ct", "boulevard", "blvd"} {
		if strings.Contains(t, word) {
			return true
		}
	}
	return false
}

func skipDetailPaneText(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return true
	}
	for _, button := range GetAppStrings().DetailButtons() {
		if t == strings.ToLower(button) {
			return true
		}
	}
	if strings.Contains(t, " updated ") || strings.HasPrefix(t, "updated ") || strings.Contains(t, " away") {
		return true
	}
	if strings.HasPrefix(t, "battery") || isBattery(t) {
		return true
	}
	for _, section := range []string{"notifications", "notify when left behind", "no location found", "offline"} {
		if t == section {
			return true
		}
	}
	return false
}
