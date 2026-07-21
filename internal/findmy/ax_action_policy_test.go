package findmy

import (
	"strings"
	"testing"
	"unicode"
)

type traversalFixtureRef struct {
	identity string
	hash     int
	children []int
}

func collectTraversalFixture(refs []traversalFixtureRef, root int) []string {
	seen := map[int][]string{}
	queue := []int{root}
	var identities []string
	for len(queue) > 0 {
		index := queue[0]
		queue = queue[1:]
		ref := refs[index]
		duplicate := false
		for _, identity := range seen[ref.hash] {
			if identity == ref.identity {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[ref.hash] = append(seen[ref.hash], ref.identity)
		identities = append(identities, ref.identity)
		queue = append(queue, ref.children...)
	}
	return identities
}

func TestTraversalPolicyDeduplicatesReferencesBreaksCyclesAndChecksHashCollisions(t *testing.T) {
	refs := []traversalFixtureRef{
		{identity: "root", hash: 1, children: []int{1, 2, 3}},
		{identity: "more-info", hash: 7, children: []int{0}},
		{identity: "more-info", hash: 7},
		{identity: "different-control", hash: 7},
	}
	got := collectTraversalFixture(refs, 0)
	want := []string{"root", "more-info", "different-control"}
	if len(got) != len(want) {
		t.Fatalf("collectTraversalFixture() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collectTraversalFixture() = %v, want %v", got, want)
		}
	}
}

type controlFixture struct {
	identity    string
	hash        int
	label       string
	identifier  string
	enabled     bool
	detailPane  bool
	press       bool
	window      string
	withinFrame bool
}

func resolveControlFixture(controls []controlFixture, labels, identifiers map[string]bool) []string {
	seen := map[int][]string{}
	var matches []string
	for _, control := range controls {
		if !control.enabled || !control.detailPane || !control.press ||
			control.window != "main" || !control.withinFrame ||
			(!labels[control.label] && !identifiers[control.identifier]) {
			continue
		}
		duplicate := false
		for _, identity := range seen[control.hash] {
			if identity == control.identity {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[control.hash] = append(seen[control.hash], control.identity)
		matches = append(matches, control.identity)
	}
	return matches
}

func TestMoreInfoAndPlaySoundPolicyDeduplicatesPhysicalControls(t *testing.T) {
	moreInfoLabels := map[string]bool{"More Info": true, "Additional Info": true, "Info": true, "추가 정보": true}
	playSoundLabels := map[string]bool{"Play Sound": true, "사운드 재생": true}

	moreInfo := resolveControlFixture([]controlFixture{
		{identity: "info-button", hash: 4, label: "추가 정보", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
		{identity: "info-button", hash: 4, label: "추가 정보", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
	}, moreInfoLabels, nil)
	if len(moreInfo) != 1 {
		t.Fatalf("More Info matches = %v, want one physical control", moreInfo)
	}

	playSound := resolveControlFixture([]controlFixture{
		{identity: "sheet-sound-button", hash: 9, label: "Play Sound", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
		{identity: "sheet-sound-button", hash: 9, label: "Play Sound", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
	}, playSoundLabels, nil)
	if len(playSound) != 1 {
		t.Fatalf("Play Sound matches = %v, want one physical control", playSound)
	}

	identifierMatch := resolveControlFixture([]controlFixture{
		{identity: "info-by-identifier", hash: 12, identifier: "moreinfobutton", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
	}, nil, map[string]bool{"moreinfobutton": true})
	if len(identifierMatch) != 1 {
		t.Fatalf("More Info identifier matches = %v, want one control", identifierMatch)
	}
}

func TestControlPolicyFailsClosedForDistinctOrInvalidControls(t *testing.T) {
	labels := map[string]bool{"More Info": true, "추가 정보": true}
	distinct := resolveControlFixture([]controlFixture{
		{identity: "first", hash: 5, label: "More Info", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
		{identity: "second", hash: 5, label: "More Info", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
	}, labels, nil)
	if len(distinct) != 2 {
		t.Fatalf("hash collision collapsed distinct controls: %v", distinct)
	}

	invalid := resolveControlFixture([]controlFixture{
		{identity: "disabled", hash: 1, label: "More Info", detailPane: true, press: true, window: "main", withinFrame: true},
		{identity: "sidebar", hash: 2, label: "More Info", enabled: true, press: true, window: "main", withinFrame: true},
		{identity: "wrong-action", hash: 3, label: "More Info", enabled: true, detailPane: true, window: "main", withinFrame: true},
		{identity: "wrong-label", hash: 4, label: "Details", enabled: true, detailPane: true, press: true, window: "main", withinFrame: true},
		{identity: "other-window", hash: 5, label: "More Info", enabled: true, detailPane: true, press: true, window: "other", withinFrame: true},
		{identity: "outside-frame", hash: 6, label: "More Info", enabled: true, detailPane: true, press: true, window: "main"},
	}, labels, nil)
	if len(invalid) != 0 {
		t.Fatalf("invalid controls matched: %v", invalid)
	}
}

func TestControlPolicyRequiresTargetWindowDescendantAndFrameContainment(t *testing.T) {
	labels := map[string]bool{"More Info": true}
	base := controlFixture{
		identity: "info", hash: 1, label: "More Info", enabled: true,
		detailPane: true, press: true, window: "main", withinFrame: true,
	}
	if got := resolveControlFixture([]controlFixture{base}, labels, nil); len(got) != 1 {
		t.Fatalf("target-window descendant = %v, want one control", got)
	}

	siblingWindow := base
	siblingWindow.identity = "sibling-info"
	siblingWindow.window = "other"
	if got := resolveControlFixture([]controlFixture{siblingWindow}, labels, nil); len(got) != 0 {
		t.Fatalf("sibling-window control matched: %v", got)
	}

	outOfFrame := base
	outOfFrame.identity = "outside-info"
	outOfFrame.withinFrame = false
	if got := resolveControlFixture([]controlFixture{outOfFrame}, labels, nil); len(got) != 0 {
		t.Fatalf("out-of-frame control matched: %v", got)
	}
}

type detailNameFixture struct {
	identity    string
	hash        int
	text        string
	identifier  string
	staticText  bool
	targetPane  bool
	withinFrame bool
}

func policySemanticNameKey(value string) string {
	var key strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			key.WriteRune(r)
		}
	}
	return key.String()
}

func resolveCanonicalDetailFixture(target string, fixtures []detailNameFixture) (string, bool) {
	targetKey := policySemanticNameKey(target)
	type candidate struct {
		text   string
		length int
	}
	var candidates []candidate
	for _, fixture := range fixtures {
		key := policySemanticNameKey(fixture.text)
		if !fixture.staticText || !fixture.targetPane || !fixture.withinFrame || key == "" ||
			(key != targetKey && !strings.HasPrefix(targetKey, key)) {
			continue
		}
		candidates = append(candidates, candidate{
			text:   strings.ToLower(strings.TrimSpace(fixture.text)),
			length: len([]rune(key)),
		})
	}
	longest := -1
	for _, candidate := range candidates {
		if candidate.length > longest {
			longest = candidate.length
		}
	}
	var winners []candidate
	for _, candidate := range candidates {
		if candidate.length == longest {
			winners = append(winners, candidate)
		}
	}
	if len(winners) != 1 {
		return "", false
	}
	return winners[0].text, true
}

func TestCanonicalDetailNameUsesUniqueLongestWithoutCommaSplitting(t *testing.T) {
	fixtures := []detailNameFixture{
		{text: "yeowool", staticText: true, targetPane: true, withinFrame: true},
		{text: "yeowool_phone", identifier: "PrimaryLabel", staticText: true, targetPane: true, withinFrame: true},
		{text: "unrelated", identifier: "PrimaryLabel", staticText: true, targetPane: true, withinFrame: true},
	}
	got, ok := resolveCanonicalDetailFixture("yeowool_phone, 집 , 지금", fixtures)
	if !ok || got != "yeowool_phone" {
		t.Fatalf("canonical detail name = %q, %v; want yeowool_phone", got, ok)
	}

	commaName := []detailNameFixture{
		{text: "Smith", staticText: true, targetPane: true, withinFrame: true},
		{text: "Smith, John", staticText: true, targetPane: true, withinFrame: true},
	}
	got, ok = resolveCanonicalDetailFixture("Smith, John, Home, Now", commaName)
	if !ok || got != "smith, john" {
		t.Fatalf("comma-containing canonical name = %q, %v; want full name", got, ok)
	}

	longestPreferred := []detailNameFixture{
		{text: "Device", identifier: "PrimaryLabel", staticText: true, targetPane: true, withinFrame: true},
		{text: "Device Pro", staticText: true, targetPane: true, withinFrame: true},
	}
	got, ok = resolveCanonicalDetailFixture("Device Pro, Home, Now", longestPreferred)
	if !ok || got != "device pro" {
		t.Fatalf("unique-longest preference = %q, %v; want device pro", got, ok)
	}
}

func TestCanonicalDetailNameFailsClosedOnTieOrWrongHierarchy(t *testing.T) {
	tied := []detailNameFixture{
		{text: "Phone-One", identifier: "PrimaryLabel", staticText: true, targetPane: true, withinFrame: true},
		{text: "Phone_One", identifier: "PrimaryLabel", staticText: true, targetPane: true, withinFrame: true},
	}
	if got, ok := resolveCanonicalDetailFixture("Phone One, Home", tied); ok {
		t.Fatalf("ambiguous canonical name accepted: %q", got)
	}
	outside := []detailNameFixture{
		{text: "Phone One", identifier: "PrimaryLabel", staticText: true, targetPane: true},
		{text: "Phone One", identifier: "PrimaryLabel", staticText: true, withinFrame: true},
	}
	if got, ok := resolveCanonicalDetailFixture("Phone One, Home", outside); ok {
		t.Fatalf("out-of-hierarchy canonical name accepted: %q", got)
	}
}

func policyMatchesStateLabel(value string, labels map[string]bool) bool {
	normalizedValue := strings.ToLower(strings.TrimSpace(value))
	for label := range labels {
		normalizedLabel := strings.ToLower(strings.TrimSpace(label))
		if normalizedValue == normalizedLabel {
			return true
		}
		if !strings.HasPrefix(normalizedValue, normalizedLabel) {
			continue
		}
		suffix := strings.TrimPrefix(normalizedValue, normalizedLabel)
		suffixRunes := []rune(suffix)
		if len(suffixRunes) == 0 || !strings.ContainsRune(",，:：", suffixRunes[0]) {
			continue
		}
		if strings.TrimSpace(string(suffixRunes[1:])) != "" {
			return true
		}
	}
	return false
}

func TestPlaySoundStateSuffixPolicyIsDelimitedAndActionSpecific(t *testing.T) {
	labels := map[string]bool{"Play Sound": true, "사운드 재생": true}
	for _, value := range []string{"Play Sound", "Play Sound, Off", "사운드 재생,끔", "Play Sound: Disabled"} {
		if !policyMatchesStateLabel(value, labels) {
			t.Errorf("stateful label %q did not match", value)
		}
	}
	for _, value := range []string{"Replay Sound", "Play Sound Off", "Play Sound,", "Play Sound/Off"} {
		if policyMatchesStateLabel(value, labels) {
			t.Errorf("unsafe label %q matched", value)
		}
	}
	if policyMatchesStateLabel("Turn Off", labels) {
		t.Fatal("generic FMPlatterButton-style control was authorized by a non-action label")
	}
}

type popoverTransitionFixture struct {
	identity       string
	hash           int
	window         string
	descendant     bool
	windowFrame    policyFrame
	frame          policyFrame
	targetLabels   []detailNameFixture
	actionLabels   []string
	actionIDs      []string
	actionsEnabled []bool
}

type policyFrame struct {
	x      float64
	y      float64
	width  float64
	height float64
}

func policyFrameContainsWithinTolerance(outer, inner policyFrame, tolerance float64) bool {
	if tolerance < 0 || tolerance > 12 || outer.width <= 0 || outer.height <= 0 ||
		inner.width <= 0 || inner.height <= 0 {
		return false
	}
	return inner.x >= outer.x-tolerance && inner.y >= outer.y-tolerance &&
		inner.x+inner.width <= outer.x+outer.width+tolerance &&
		inner.y+inner.height <= outer.y+outer.height+tolerance
}

func TestPopoverWindowFrameToleranceObservedBoundary(t *testing.T) {
	window := policyFrame{x: 896, y: 234, width: 1024, height: 768}
	observedPopover := policyFrame{x: 1558, y: 252, width: 318, height: 754}
	if !policyFrameContainsWithinTolerance(window, observedPopover, 8) {
		t.Fatal("observed 4 px Catalyst popover spill was rejected")
	}
	beyondTolerance := observedPopover
	beyondTolerance.height = 759
	if policyFrameContainsWithinTolerance(window, beyondTolerance, 8) {
		t.Fatal("9 px popover spill was accepted")
	}
}

func validatePopoverTransitionFixture(
	windowIdentity, canonicalName string,
	popovers []popoverTransitionFixture,
	labels map[string]bool,
) bool {
	seen := map[int][]string{}
	var matching []popoverTransitionFixture
	for _, popover := range popovers {
		if popover.window != windowIdentity || !popover.descendant ||
			!policyFrameContainsWithinTolerance(popover.windowFrame, popover.frame, 8) {
			continue
		}
		duplicate := false
		for _, identity := range seen[popover.hash] {
			if identity == popover.identity {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[popover.hash] = append(seen[popover.hash], popover.identity)
		matching = append(matching, popover)
	}
	if len(matching) != 1 {
		return false
	}
	popover := matching[0]
	seenTargets := map[int][]string{}
	var targetMatches int
	for _, label := range popover.targetLabels {
		if !label.staticText || !label.targetPane || !label.withinFrame ||
			!strings.EqualFold(strings.TrimSpace(label.text), canonicalName) {
			continue
		}
		duplicate := false
		for _, identity := range seenTargets[label.hash] {
			if identity == label.identity {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seenTargets[label.hash] = append(seenTargets[label.hash], label.identity)
		targetMatches++
	}
	if targetMatches != 1 {
		return false
	}

	actionMatches := 0
	for i, label := range popover.actionLabels {
		enabled := i < len(popover.actionsEnabled) && popover.actionsEnabled[i]
		identifier := ""
		if i < len(popover.actionIDs) {
			identifier = strings.ToLower(popover.actionIDs[i])
		}
		actionSpecificID := identifier == "playsound" || identifier == "playsoundbutton"
		if enabled && (actionSpecificID || policyMatchesStateLabel(label, labels)) {
			actionMatches++
		}
	}
	return actionMatches == 1
}

func TestPopoverTransitionBindsWindowTargetAndUniqueAction(t *testing.T) {
	labels := map[string]bool{"Play Sound": true, "사운드 재생": true}
	windowFrame := policyFrame{x: 896, y: 234, width: 1024, height: 768}
	popoverFrame := policyFrame{x: 1558, y: 252, width: 318, height: 754}
	valid := popoverTransitionFixture{
		identity: "popover", hash: 7, window: "selected-window", descendant: true,
		windowFrame: windowFrame, frame: popoverFrame,
		targetLabels: []detailNameFixture{{
			identity: "target-label", hash: 11, text: "yeowool_phone",
			identifier: "PrimaryLabel", staticText: true,
			targetPane: true, withinFrame: true,
		}},
		actionLabels: []string{"사운드 재생,끔"}, actionIDs: []string{"FMPlatterButton"},
		actionsEnabled: []bool{true},
	}
	if !validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{valid}, labels) {
		t.Fatal("valid identity-bound popover transition failed")
	}

	sibling := valid
	sibling.window = "other-window"
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{sibling}, labels) {
		t.Fatal("sibling-window popover was accepted")
	}
	notDescendant := valid
	notDescendant.descendant = false
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{notDescendant}, labels) {
		t.Fatal("geometry authorized a popover without window ancestry")
	}
	beyondTolerance := valid
	beyondTolerance.frame.height = 759
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{beyondTolerance}, labels) {
		t.Fatal("popover spilling beyond the 8 px tolerance was accepted")
	}
	if !validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{valid, valid}, labels) {
		t.Fatal("duplicate references to one popover were not deduplicated")
	}
	distinctPopover := valid
	distinctPopover.identity = "second-popover"
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{valid, distinctPopover}, labels) {
		t.Fatal("multiple popovers were accepted")
	}

	duplicateTarget := valid
	duplicateTarget.targetLabels = append(duplicateTarget.targetLabels, duplicateTarget.targetLabels[0])
	if !validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{duplicateTarget}, labels) {
		t.Fatal("duplicate references to one target label were not deduplicated")
	}
	ambiguousTarget := valid
	ambiguousTarget.targetLabels = append(ambiguousTarget.targetLabels, detailNameFixture{
		identity: "second-target-label", hash: 11, text: "yeowool_phone",
		staticText: true, targetPane: true, withinFrame: true,
	})
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{ambiguousTarget}, labels) {
		t.Fatal("PrimaryLabel suppressed a second exact target label")
	}
	ambiguousAction := valid
	ambiguousAction.actionLabels = append(ambiguousAction.actionLabels, "Play Sound, Off")
	ambiguousAction.actionIDs = append(ambiguousAction.actionIDs, "playsoundbutton")
	ambiguousAction.actionsEnabled = append(ambiguousAction.actionsEnabled, true)
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{ambiguousAction}, labels) {
		t.Fatal("ambiguous Play Sound actions were accepted")
	}
	wrongGenericAction := valid
	wrongGenericAction.actionLabels = []string{"Turn Off"}
	if validatePopoverTransitionFixture("selected-window", "yeowool_phone", []popoverTransitionFixture{wrongGenericAction}, labels) {
		t.Fatal("generic button identifier authorized a non-Play Sound action")
	}
}
