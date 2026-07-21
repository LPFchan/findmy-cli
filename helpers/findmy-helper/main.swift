import AppKit
import ApplicationServices
import Foundation

func die(_ message: String, code: Int32 = 1) -> Never {
    FileHandle.standardError.write(Data((message + "\n").utf8))
    exit(code)
}

func emit<T: Encodable>(_ value: T) {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    do {
        let data = try encoder.encode(value)
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data([0x0a]))
    } catch {
        die("encode failed: \(error)")
    }
}

struct Permissions: Encodable {
    let accessibility: Bool
}

struct AXFrame: Encodable {
    let x: Double
    let y: Double
    let width: Double
    let height: Double
}

struct AXNode: Encodable {
    let path: String
    let parentPath: String?
    let role: String
    let subrole: String
    let title: String
    let value: String
    let description: String
    let identifier: String
    let selected: Bool?
    let enabled: Bool?
    let frame: AXFrame?
    let actions: [String]
}

struct AXTree: Encodable {
    let pid: Int32
    let nodes: [AXNode]
}

struct ActionResult: Encodable {
    let ok: Bool
    let action: String
    let target: String
}

struct AXSnapshot {
    let pid: Int32
    let nodes: [AXNode]
    let elements: [String: AXUIElement]
    let nodesByPath: [String: AXNode]
}

struct AXRowMatch {
    let semanticPath: String
    let exactTextPaths: [String]
}

struct AXNameMatch {
    let text: String
    let keyLength: Int
}

func copyAttribute(_ element: AXUIElement, _ attribute: String) -> CFTypeRef? {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, attribute as CFString, &value) == .success else {
        return nil
    }
    return value
}

func stringAttribute(_ element: AXUIElement, _ attribute: String) -> String {
    guard let value = copyAttribute(element, attribute) else { return "" }
    if let string = value as? String { return string }
    if let number = value as? NSNumber { return number.stringValue }
    if let attributed = value as? NSAttributedString { return attributed.string }
    return ""
}

func boolAttribute(_ element: AXUIElement, _ attribute: String) -> Bool? {
    guard let value = copyAttribute(element, attribute) else { return nil }
    return (value as? NSNumber)?.boolValue
}

func children(_ element: AXUIElement) -> [AXUIElement] {
    guard let value = copyAttribute(element, kAXChildrenAttribute as String) else { return [] }
    return value as? [AXUIElement] ?? []
}

func actionNames(_ element: AXUIElement) -> [String] {
    var names: CFArray?
    guard AXUIElementCopyActionNames(element, &names) == .success else { return [] }
    return names as? [String] ?? []
}

func frame(_ element: AXUIElement) -> AXFrame? {
    guard let positionValue = copyAttribute(element, kAXPositionAttribute as String),
          let sizeValue = copyAttribute(element, kAXSizeAttribute as String),
          CFGetTypeID(positionValue) == AXValueGetTypeID(),
          CFGetTypeID(sizeValue) == AXValueGetTypeID() else { return nil }

    let position = unsafeBitCast(positionValue, to: AXValue.self)
    let size = unsafeBitCast(sizeValue, to: AXValue.self)
    var point = CGPoint.zero
    var dimensions = CGSize.zero
    guard AXValueGetValue(position, .cgPoint, &point),
          AXValueGetValue(size, .cgSize, &dimensions) else { return nil }
    return AXFrame(
        x: Double(point.x), y: Double(point.y),
        width: Double(dimensions.width), height: Double(dimensions.height)
    )
}

func runningFindMy() -> NSRunningApplication? {
    for identifier in ["com.apple.findmy", "com.apple.FindMy"] {
        if let app = NSRunningApplication.runningApplications(withBundleIdentifier: identifier).first {
            return app
        }
    }
    return NSWorkspace.shared.runningApplications.first {
        $0.bundleURL?.lastPathComponent == "FindMy.app"
    }
}

func findMyRoot() -> AXUIElement {
    guard AXIsProcessTrusted() else {
        die("Accessibility permission is required. Grant access in System Settings → Privacy & Security → Accessibility, then relaunch the caller.")
    }
    guard let app = runningFindMy() else { die("Find My is not running") }
    let root = AXUIElementCreateApplication(app.processIdentifier)
    AXUIElementSetMessagingTimeout(root, 5.0)
    return root
}

func claimUniqueElement(_ element: AXUIElement, buckets: inout [CFHashCode: [AXUIElement]]) -> Bool {
    let hash = CFHash(element)
    if buckets[hash]?.contains(where: { CFEqual($0, element) }) == true {
        return false
    }
    buckets[hash, default: []].append(element)
    return true
}

func collectTree(_ root: AXUIElement, maxDepth: Int = 32, maxNodes: Int = 10_000) -> AXSnapshot {
    var pid: pid_t = 0
    AXUIElementGetPid(root, &pid)
    var nodes: [AXNode] = []
    var elements: [String: AXUIElement] = [:]
    var nodesByPath: [String: AXNode] = [:]
    var seenElements: [CFHashCode: [AXUIElement]] = [:]
    var queue: [(element: AXUIElement, path: String, parentPath: String?, depth: Int)] = [
        (root, "", nil, 0)
    ]
    var queueIndex = 0

    while queueIndex < queue.count && nodes.count < maxNodes {
        let item = queue[queueIndex]
        queueIndex += 1
        guard item.depth <= maxDepth,
              claimUniqueElement(item.element, buckets: &seenElements) else { continue }
        let node = AXNode(
            path: item.path,
            parentPath: item.parentPath,
            role: stringAttribute(item.element, kAXRoleAttribute as String),
            subrole: stringAttribute(item.element, kAXSubroleAttribute as String),
            title: stringAttribute(item.element, kAXTitleAttribute as String),
            value: stringAttribute(item.element, kAXValueAttribute as String),
            description: stringAttribute(item.element, kAXDescriptionAttribute as String),
            identifier: stringAttribute(item.element, kAXIdentifierAttribute as String),
            selected: boolAttribute(item.element, kAXSelectedAttribute as String),
            enabled: boolAttribute(item.element, kAXEnabledAttribute as String),
            frame: frame(item.element),
            actions: actionNames(item.element)
        )
        nodes.append(node)
        elements[item.path] = item.element
        nodesByPath[item.path] = node

        for (index, child) in children(item.element).enumerated() {
            let childPath = item.path.isEmpty ? String(index) : "\(item.path).\(index)"
            queue.append((child, childPath, item.path, item.depth + 1))
        }
    }
    return AXSnapshot(pid: pid, nodes: nodes, elements: elements, nodesByPath: nodesByPath)
}

func normalized(_ text: String) -> String {
    text.trimmingCharacters(in: .whitespacesAndNewlines)
        .folding(options: [.caseInsensitive], locale: Locale(identifier: "en_US_POSIX"))
}

func nodeTexts(_ node: AXNode) -> [String] {
    [node.title, node.value, node.description].filter { !$0.isEmpty }
}

func nodeHasExactText(_ node: AXNode, wanted: String) -> Bool {
    nodeTexts(node).contains { normalized($0) == wanted }
}

func hasAction(_ node: AXNode, _ action: String) -> Bool {
    node.actions.contains(action)
}

func press(_ element: AXUIElement) -> AXError {
    let actions = actionNames(element)
    if actions.contains(kAXPressAction as String) {
        return AXUIElementPerformAction(element, kAXPressAction as CFString)
    }
    if actions.contains(kAXPickAction as String) {
        return AXUIElementPerformAction(element, kAXPickAction as CFString)
    }
    return .actionUnsupported
}

func mainWindow(_ snapshot: AXSnapshot) -> AXNode? {
    snapshot.nodes
        .filter { $0.role == "AXWindow" && $0.frame != nil }
        .max { left, right in
            let leftFrame = left.frame!
            let rightFrame = right.frame!
            return leftFrame.width * leftFrame.height < rightFrame.width * rightFrame.height
        }
}

func isSemanticRow(_ node: AXNode) -> Bool {
    switch node.role {
    case "AXRow", "AXCell", "AXListItem":
        return true
    case "AXGroup":
        return node.selected != nil || hasAction(node, "AXPress") || hasAction(node, "AXPick")
    default:
        return false
    }
}

func nearestSemanticRowPath(_ node: AXNode, snapshot: AXSnapshot) -> String? {
    var parentPath = node.parentPath
    while let path = parentPath, let parent = snapshot.nodesByPath[path] {
        if isSemanticRow(parent) { return path }
        parentPath = parent.parentPath
    }
    return isSemanticRow(node) ? node.path : nil
}

func nearestPressPath(_ node: AXNode, snapshot: AXSnapshot) -> String? {
    var candidate: AXNode? = node
    while let current = candidate {
        if current.role == "AXButton" && hasAction(current, "AXPress") { return current.path }
        guard let parentPath = current.parentPath else { return nil }
        candidate = snapshot.nodesByPath[parentPath]
    }
    return nil
}

func exactSidebarRows(_ snapshot: AXSnapshot, target: String) -> [AXRowMatch] {
    guard let windowFrame = mainWindow(snapshot)?.frame else {
        die("Find My Accessibility tree has no main window")
    }
    let sidebarRight = windowFrame.x + min(480, windowFrame.width * 0.48)
    var rowOrder: [String] = []
    var textPathsByRow: [String: [String]] = [:]
    for node in snapshot.nodes {
        guard node.role == "AXStaticText", let nodeFrame = node.frame,
              nodeHasExactText(node, wanted: target),
              nodeFrame.x < sidebarRight, nodeFrame.y >= windowFrame.y + 45 else { continue }
        guard let rowPath = nearestSemanticRowPath(node, snapshot: snapshot) else { continue }
        if textPathsByRow[rowPath] == nil {
            rowOrder.append(rowPath)
            textPathsByRow[rowPath] = []
        }
        if textPathsByRow[rowPath]?.contains(node.path) == false {
            textPathsByRow[rowPath]?.append(node.path)
        }
    }
    return rowOrder.map {
        AXRowMatch(semanticPath: $0, exactTextPaths: textPathsByRow[$0] ?? [])
    }
}

func isSelectionActionable(_ node: AXNode) -> Bool {
    node.enabled != false && (hasAction(node, "AXPress") || hasAction(node, "AXPick"))
}

func isDescendant(_ path: String, of ancestorPath: String, snapshot: AXSnapshot) -> Bool {
    var currentPath = snapshot.nodesByPath[path]?.parentPath
    while let current = currentPath {
        if current == ancestorPath { return true }
        currentPath = snapshot.nodesByPath[current]?.parentPath
    }
    return false
}

func ancestorWindow(_ path: String, snapshot: AXSnapshot) -> AXNode? {
    var currentPath: String? = path
    while let current = currentPath, let node = snapshot.nodesByPath[current] {
        if node.role == "AXWindow" { return node }
        currentPath = node.parentPath
    }
    return nil
}

func exactTargetWindow(_ snapshot: AXSnapshot, target: String) -> AXNode? {
    let rows = exactSidebarRows(snapshot, target: target)
    guard rows.count == 1 else { return nil }
    return ancestorWindow(rows[0].semanticPath, snapshot: snapshot)
}

func selectionPressPath(_ match: AXRowMatch, snapshot: AXSnapshot) -> (path: String?, failure: String?) {
    guard let row = snapshot.nodesByPath[match.semanticPath], isSemanticRow(row) else {
        return (nil, "matched path is not a semantic Accessibility row")
    }
    let exactActions = match.exactTextPaths.filter {
        guard let node = snapshot.nodesByPath[$0] else { return false }
        return isDescendant($0, of: match.semanticPath, snapshot: snapshot) &&
            isSelectionActionable(node)
    }
    if exactActions.count == 1 { return (exactActions[0], nil) }
    if exactActions.count > 1 {
        return (nil, "exact target text exposes multiple actionable elements")
    }

    let descendantActions = snapshot.nodes.filter {
        $0.path != match.semanticPath &&
            isDescendant($0.path, of: match.semanticPath, snapshot: snapshot) &&
            isSelectionActionable($0)
    }.map(\.path)
    if descendantActions.count == 1 { return (descendantActions[0], nil) }
    if descendantActions.count > 1 {
        return (nil, "semantic row exposes multiple actionable descendants")
    }

    guard isSelectionActionable(row) else {
        return (nil, "semantic row exposes no actionable selection element")
    }
    return (match.semanticPath, nil)
}

func verifySelectedTarget(_ snapshot: AXSnapshot, target: String) -> String? {
    let targetRows = exactSidebarRows(snapshot, target: target)
    guard targetRows.count == 1 else {
        return "target verification found \(targetRows.count) exact sidebar rows"
    }

    let selectedRows = snapshot.nodes.filter { isSemanticRow($0) && $0.selected == true }.map(\.path)
    if !selectedRows.isEmpty {
        guard selectedRows.count == 1, selectedRows[0] == targetRows[0].semanticPath else {
            return "a different or ambiguous Accessibility row is selected"
        }
        return nil
    }

    guard let windowFrame = mainWindow(snapshot)?.frame else {
        return "Find My Accessibility tree has no main window"
    }
    let sidebarRight = windowFrame.x + min(480, windowFrame.width * 0.48)
    let detailMatches = snapshot.nodes.filter {
        $0.role == "AXStaticText" && ($0.frame?.x ?? -.infinity) >= sidebarRight &&
            nodeHasExactText($0, wanted: target)
    }
    guard detailMatches.count == 1 else {
        return "detail verification found \(detailMatches.count) exact target labels"
    }
    return nil
}

func canonicalIdentifier(_ identifier: String) -> String {
    normalized(identifier).filter { $0.isLetter || $0.isNumber }
}

func semanticNameKey(_ text: String) -> String {
    normalized(text).unicodeScalars.compactMap {
        CharacterSet.alphanumerics.contains($0) ? String($0) : nil
    }.joined()
}

func labelBindsToCompositeTarget(_ label: String, target: String) -> Bool {
    let labelKey = semanticNameKey(label)
    let targetKey = semanticNameKey(target)
    return !labelKey.isEmpty && (labelKey == targetKey || targetKey.hasPrefix(labelKey))
}

func matchesAllowedStateLabel(_ text: String, labels: Set<String>) -> Bool {
    let value = normalized(text)
    let allowedDelimiters: Set<Character> = [",", "，", ":", "："]
    for label in labels {
        if value == label { return true }
        guard value.hasPrefix(label) else { continue }
        let suffix = value.dropFirst(label.count)
        guard let delimiter = suffix.first, allowedDelimiters.contains(delimiter) else { continue }
        if !suffix.dropFirst().trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return true
        }
    }
    return false
}

func isControlNode(
    _ node: AXNode,
    labels: Set<String>,
    identifiers: Set<String>,
    allowStateSuffix: Bool = false
) -> Bool {
    if identifiers.contains(canonicalIdentifier(node.identifier)) { return true }
    if allowStateSuffix {
        return nodeTexts(node).contains { matchesAllowedStateLabel($0, labels: labels) }
    }
    return nodeTexts(node).contains { labels.contains(normalized($0)) }
}

func frameContains(_ outer: AXFrame, _ inner: AXFrame) -> Bool {
    guard outer.width > 0, outer.height > 0, inner.width > 0, inner.height > 0 else { return false }
    return inner.x >= outer.x && inner.y >= outer.y &&
        inner.x + inner.width <= outer.x + outer.width &&
        inner.y + inner.height <= outer.y + outer.height
}

func frameContainsWithinEdgeTolerance(_ outer: AXFrame, _ inner: AXFrame, tolerance: Double) -> Bool {
    guard tolerance >= 0, tolerance <= 12,
          outer.width > 0, outer.height > 0, inner.width > 0, inner.height > 0 else { return false }
    return inner.x >= outer.x - tolerance && inner.y >= outer.y - tolerance &&
        inner.x + inner.width <= outer.x + outer.width + tolerance &&
        inner.y + inner.height <= outer.y + outer.height + tolerance
}

func matchingControlPaths(
    _ snapshot: AXSnapshot,
    window: AXNode,
    sidebarRight: Double,
    labels: Set<String>,
    identifiers: Set<String>,
    allowStateSuffix: Bool = false
) -> [String] {
    guard window.role == "AXWindow", let windowFrame = window.frame else { return [] }
    var paths: [String] = []
    var seenElements: [CFHashCode: [AXUIElement]] = [:]
    for node in snapshot.nodes where isControlNode(
        node, labels: labels, identifiers: identifiers, allowStateSuffix: allowStateSuffix
    ) {
        guard let path = nearestPressPath(node, snapshot: snapshot),
              let controlNode = snapshot.nodesByPath[path],
              controlNode.role == "AXButton",
              hasAction(controlNode, "AXPress"),
              controlNode.enabled != false,
              isDescendant(path, of: window.path, snapshot: snapshot),
              let controlFrame = controlNode.frame,
              controlFrame.x >= sidebarRight,
              frameContains(windowFrame, controlFrame),
              let control = snapshot.elements[path],
              claimUniqueElement(control, buckets: &seenElements) else { continue }
        paths.append(path)
    }
    return paths
}

func canonicalDetailName(
    _ snapshot: AXSnapshot,
    window: AXNode,
    sidebarRight: Double,
    compositeTarget: String
) -> (name: String?, failure: String?) {
    guard window.role == "AXWindow", let windowFrame = window.frame else {
        return (nil, "selected device window has no usable frame")
    }
    var candidates: [AXNameMatch] = []
    for node in snapshot.nodes {
        guard node.role == "AXStaticText",
              isDescendant(node.path, of: window.path, snapshot: snapshot),
              let nodeFrame = node.frame,
              nodeFrame.x >= sidebarRight,
              frameContains(windowFrame, nodeFrame) else { continue }
        var seenTexts = Set<String>()
        for text in nodeTexts(node) {
            let value = normalized(text)
            guard seenTexts.insert(value).inserted,
                  labelBindsToCompositeTarget(value, target: compositeTarget) else { continue }
            candidates.append(AXNameMatch(
                text: value,
                keyLength: semanticNameKey(value).count
            ))
        }
    }
    guard let longest = candidates.map(\.keyLength).max() else {
        return (nil, "detail pane has no label bound to the selected composite target")
    }
    let winners = candidates.filter { $0.keyLength == longest }
    guard winners.count == 1 else {
        return (nil, "detail pane has \(winners.count) equally specific target labels")
    }
    return (winners[0].text, nil)
}

func sameUnderlyingWindow(_ element: AXUIElement, snapshot: AXSnapshot) -> AXNode? {
    let expectedHash = CFHash(element)
    let matches = snapshot.nodes.filter { node in
        guard node.role == "AXWindow", let candidate = snapshot.elements[node.path] else { return false }
        return CFHash(candidate) == expectedHash && CFEqual(candidate, element)
    }
    return matches.count == 1 ? matches[0] : nil
}

func matchingPopoverPaths(_ snapshot: AXSnapshot, window: AXNode) -> [String] {
    guard let windowFrame = window.frame else { return [] }
    var paths: [String] = []
    var seenElements: [CFHashCode: [AXUIElement]] = [:]
    for node in snapshot.nodes {
        guard node.role == "AXPopover",
              isDescendant(node.path, of: window.path, snapshot: snapshot),
              let popoverFrame = node.frame,
              frameContainsWithinEdgeTolerance(windowFrame, popoverFrame, tolerance: 8),
              let element = snapshot.elements[node.path],
              claimUniqueElement(element, buckets: &seenElements) else { continue }
        paths.append(node.path)
    }
    return paths
}

func matchingPopoverTargetPaths(
    _ snapshot: AXSnapshot,
    popover: AXNode,
    canonicalName: String
) -> [String] {
    guard let popoverFrame = popover.frame else { return [] }
    var paths: [String] = []
    var seenElements: [CFHashCode: [AXUIElement]] = [:]
    for node in snapshot.nodes {
        guard node.role == "AXStaticText",
              isDescendant(node.path, of: popover.path, snapshot: snapshot),
              let nodeFrame = node.frame,
              frameContains(popoverFrame, nodeFrame),
              nodeTexts(node).contains(where: { normalized($0) == canonicalName }),
              let element = snapshot.elements[node.path],
              claimUniqueElement(element, buckets: &seenElements) else { continue }
        paths.append(node.path)
    }
    return paths
}

func matchingPopoverPlaySoundPaths(
    _ snapshot: AXSnapshot,
    window: AXNode,
    popover: AXNode,
    labels: Set<String>,
    identifiers: Set<String>
) -> [String] {
    guard let windowFrame = window.frame, let popoverFrame = popover.frame else { return [] }
    var paths: [String] = []
    var seenElements: [CFHashCode: [AXUIElement]] = [:]
    for node in snapshot.nodes {
        guard node.role == "AXButton",
              node.enabled != false,
              hasAction(node, "AXPress"),
              isDescendant(node.path, of: popover.path, snapshot: snapshot),
              let nodeFrame = node.frame,
              frameContains(windowFrame, nodeFrame),
              frameContains(popoverFrame, nodeFrame),
              isControlNode(node, labels: labels, identifiers: identifiers, allowStateSuffix: true),
              let element = snapshot.elements[node.path],
              claimUniqueElement(element, buckets: &seenElements) else { continue }
        paths.append(node.path)
    }
    return paths
}

func option(_ args: [String], _ name: String) -> String? {
    guard let index = args.firstIndex(of: name), args.indices.contains(index + 1) else { return nil }
    return args[index + 1]
}

func options(_ args: [String], _ name: String) -> [String] {
    var values: [String] = []
    var index = 0
    while index + 1 < args.count {
        if args[index] == name {
            values.append(args[index + 1])
            index += 2
        } else {
            index += 1
        }
    }
    return values
}

func selectTab(_ root: AXUIElement, label: String) -> AXError {
    let wanted = normalized(label)
    let snapshot = collectTree(root)
    let matches = snapshot.nodes.filter { node in
        let identifier = normalized(node.identifier)
        let safeRole = node.role == "AXRadioButton" ||
            ((node.role == "AXButton" || node.role == "AXMenuItem") && identifier.contains("tab"))
        return safeRole && nodeHasExactText(node, wanted: wanted) &&
            (hasAction(node, "AXPress") || hasAction(node, "AXPick"))
    }
    guard matches.count == 1,
          let element = snapshot.elements[matches[0].path] else { return .noValue }
    return press(element)
}

func cmdPermissions() {
    emit(Permissions(accessibility: AXIsProcessTrusted()))
}

func cmdTree() {
    let root = findMyRoot()
    let snapshot = collectTree(root)
    emit(AXTree(pid: snapshot.pid, nodes: snapshot.nodes))
}

func cmdSelectTab(_ args: [String]) {
    guard let label = option(args, "--label") else {
        die("usage: findmy-helper select-tab --label <localized tab label>")
    }
    let result = selectTab(findMyRoot(), label: label)
    guard result == .success else { die("could not select tab \(label): AXError \(result.rawValue)") }
    emit(ActionResult(ok: true, action: "select-tab", target: label))
}

func cmdSelectRecord(_ args: [String]) {
    guard let target = option(args, "--target"), let tabLabel = option(args, "--tab-label") else {
        die("usage: findmy-helper select-record --target <exact name> --tab-label <localized label>")
    }
    let root = findMyRoot()
    let tabResult = selectTab(root, label: tabLabel)
    guard tabResult == .success else {
        die("could not select tab \(tabLabel): AXError \(tabResult.rawValue)")
    }
    usleep(700_000)
    let snapshot = collectTree(root)
    let rows = exactSidebarRows(snapshot, target: normalized(target))
    guard rows.count == 1 else {
        die("expected one exact record row for \(target), found \(rows.count)")
    }
    let selection = selectionPressPath(rows[0], snapshot: snapshot)
    guard let selectionPath = selection.path,
          let element = snapshot.elements[selectionPath] else {
        die("could not resolve exact record \(target) selection: \(selection.failure ?? "missing Accessibility element")")
    }
    let result = press(element)
    guard result == .success else {
        die("could not select exact record \(target): AXError \(result.rawValue)")
    }
    emit(ActionResult(ok: true, action: "select-record", target: target))
}

func cmdPlaySound(_ args: [String]) {
    guard let target = option(args, "--target"),
          let devicesLabel = option(args, "--devices-label"),
          args.contains("--confirmed") else {
        die("usage: findmy-helper play-sound --target <exact name> --devices-label <localized label> --confirmed [--label <localized label>] ...")
    }
    let labels = Set(["Play Sound", "사운드 재생"].map(normalized))
    let requestedLabels = Set(options(args, "--label").map(normalized))
    guard requestedLabels.isSubset(of: labels) else {
        die("unsupported Play Sound accessibility label")
    }
    let playSoundIdentifiers: Set<String> = ["playsound", "playsoundbutton"]
    let moreInfoLabels = Set(["More Info", "Additional Info", "Info", "추가 정보"].map(normalized))
    let moreInfoIdentifiers: Set<String> = [
        "moreinfo", "moreinfobutton", "additionalinfo", "additionalinfobutton", "info", "infobutton"
    ]
    let wanted = normalized(target)

    let root = findMyRoot()
    let tabResult = selectTab(root, label: devicesLabel)
    guard tabResult == .success else {
        die("could not select Devices tab \(devicesLabel): AXError \(tabResult.rawValue)")
    }
    usleep(700_000)

    let initial = collectTree(root)
    let rows = exactSidebarRows(initial, target: wanted)
    guard rows.count == 1 else {
        die("expected one exact device row for \(target), found \(rows.count)")
    }
    let selection = selectionPressPath(rows[0], snapshot: initial)
    guard let selectionPath = selection.path,
          let rowElement = initial.elements[selectionPath] else {
        die("could not resolve exact device \(target) selection: \(selection.failure ?? "missing Accessibility element")")
    }
    let selected = press(rowElement)
    guard selected == .success else {
        die("could not select exact device \(target): AXError \(selected.rawValue)")
    }
    usleep(700_000)

    let refreshed = collectTree(root)
    if let failure = verifySelectedTarget(refreshed, target: wanted) {
        die("selected device verification failed for \(target): \(failure)")
    }
    guard let actionWindow = exactTargetWindow(refreshed, target: wanted),
          let windowFrame = actionWindow.frame else {
        die("could not identify the selected device window")
    }
    var actionSnapshot = refreshed
    let sidebarRight = windowFrame.x + min(480, windowFrame.width * 0.48)
    var controlPaths = matchingControlPaths(
        actionSnapshot,
        window: actionWindow,
        sidebarRight: sidebarRight,
        labels: labels,
        identifiers: playSoundIdentifiers,
        allowStateSuffix: true
    )
    guard controlPaths.count <= 1 else {
        die("expected at most one enabled Play Sound AX control for \(target), found \(controlPaths.count)")
    }

    if controlPaths.isEmpty {
        let detailName = canonicalDetailName(
            actionSnapshot,
            window: actionWindow,
            sidebarRight: sidebarRight,
            compositeTarget: wanted
        )
        guard let canonicalName = detailName.name else {
            die("could not capture selected device detail label: \(detailName.failure ?? "unknown error")")
        }
        guard let targetWindowElement = actionSnapshot.elements[actionWindow.path] else {
            die("could not capture selected device window identity")
        }
        let moreInfoPaths = matchingControlPaths(
            actionSnapshot,
            window: actionWindow,
            sidebarRight: sidebarRight,
            labels: moreInfoLabels,
            identifiers: moreInfoIdentifiers
        )
        guard moreInfoPaths.count == 1,
              let moreInfoControl = actionSnapshot.elements[moreInfoPaths[0]] else {
            die("expected one enabled More Info AX control for \(target), found \(moreInfoPaths.count)")
        }
        let moreInfoResult = AXUIElementPerformAction(moreInfoControl, kAXPressAction as CFString)
        guard moreInfoResult == .success else {
            die("More Info AX action failed: AXError \(moreInfoResult.rawValue)")
        }
        usleep(700_000)

        actionSnapshot = collectTree(root)
        guard let detailWindow = sameUnderlyingWindow(targetWindowElement, snapshot: actionSnapshot) else {
            die("could not reidentify the selected device window after More Info")
        }
        let popoverPaths = matchingPopoverPaths(actionSnapshot, window: detailWindow)
        guard popoverPaths.count == 1,
              let popover = actionSnapshot.nodesByPath[popoverPaths[0]] else {
            die("expected one AX popover in the selected device window, found \(popoverPaths.count)")
        }
        let targetPaths = matchingPopoverTargetPaths(
            actionSnapshot,
            popover: popover,
            canonicalName: canonicalName
        )
        guard targetPaths.count == 1 else {
            die("expected one canonical device label in the selected device popover, found \(targetPaths.count)")
        }
        controlPaths = matchingPopoverPlaySoundPaths(
            actionSnapshot,
            window: detailWindow,
            popover: popover,
            labels: labels,
            identifiers: playSoundIdentifiers
        )
    }
    guard controlPaths.count == 1,
          let control = actionSnapshot.elements[controlPaths[0]] else {
        die("expected one enabled Play Sound AX control for \(target), found \(controlPaths.count)")
    }
    let result = AXUIElementPerformAction(control, kAXPressAction as CFString)
    guard result == .success else { die("Play Sound AX action failed: AXError \(result.rawValue)") }
    emit(ActionResult(ok: true, action: "play-sound", target: target))
}

let arguments = Array(CommandLine.arguments.dropFirst())
guard let command = arguments.first else {
    die("usage: findmy-helper {tree|select-tab|select-record|play-sound|permissions} ...")
}
let rest = Array(arguments.dropFirst())
switch command {
case "tree": cmdTree()
case "select-tab": cmdSelectTab(rest)
case "select-record": cmdSelectRecord(rest)
case "play-sound": cmdPlaySound(rest)
case "permissions": cmdPermissions()
default: die("unknown subcommand: \(command)")
}
