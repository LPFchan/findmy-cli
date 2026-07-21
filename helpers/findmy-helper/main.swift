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

func collectTree(_ root: AXUIElement, maxDepth: Int = 32, maxNodes: Int = 10_000) -> AXSnapshot {
    var pid: pid_t = 0
    AXUIElementGetPid(root, &pid)
    var nodes: [AXNode] = []
    var elements: [String: AXUIElement] = [:]
    var nodesByPath: [String: AXNode] = [:]

    func visit(_ element: AXUIElement, path: String, parentPath: String?, depth: Int) {
        guard nodes.count < maxNodes, depth <= maxDepth else { return }
        let node = AXNode(
            path: path,
            parentPath: parentPath,
            role: stringAttribute(element, kAXRoleAttribute as String),
            subrole: stringAttribute(element, kAXSubroleAttribute as String),
            title: stringAttribute(element, kAXTitleAttribute as String),
            value: stringAttribute(element, kAXValueAttribute as String),
            description: stringAttribute(element, kAXDescriptionAttribute as String),
            identifier: stringAttribute(element, kAXIdentifierAttribute as String),
            selected: boolAttribute(element, kAXSelectedAttribute as String),
            enabled: boolAttribute(element, kAXEnabledAttribute as String),
            frame: frame(element),
            actions: actionNames(element)
        )
        nodes.append(node)
        elements[path] = element
        nodesByPath[path] = node

        for (index, child) in children(element).enumerated() {
            let childPath = path.isEmpty ? String(index) : "\(path).\(index)"
            visit(child, path: childPath, parentPath: path, depth: depth + 1)
        }
    }

    visit(root, path: "", parentPath: nil, depth: 0)
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

func exactSidebarRows(_ snapshot: AXSnapshot, target: String) -> [String] {
    guard let windowFrame = mainWindow(snapshot)?.frame else {
        die("Find My Accessibility tree has no main window")
    }
    let sidebarRight = windowFrame.x + min(480, windowFrame.width * 0.48)
    var paths: [String] = []
    var seen = Set<String>()
    for node in snapshot.nodes {
        guard node.role == "AXStaticText", let nodeFrame = node.frame,
              nodeHasExactText(node, wanted: target),
              nodeFrame.x < sidebarRight, nodeFrame.y >= windowFrame.y + 45 else { continue }
        guard let path = nearestSemanticRowPath(node, snapshot: snapshot) else { continue }
        if seen.insert(path).inserted { paths.append(path) }
    }
    return paths
}

func verifySelectedTarget(_ snapshot: AXSnapshot, target: String) -> String? {
    let targetRows = exactSidebarRows(snapshot, target: target)
    guard targetRows.count == 1 else {
        return "target verification found \(targetRows.count) exact sidebar rows"
    }

    let selectedRows = snapshot.nodes.filter { isSemanticRow($0) && $0.selected == true }.map(\.path)
    if !selectedRows.isEmpty {
        guard selectedRows.count == 1, selectedRows[0] == targetRows[0] else {
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

func isPlaySoundNode(_ node: AXNode, labels: Set<String>) -> Bool {
    let identifier = normalized(node.identifier).filter { $0.isLetter || $0.isNumber }
    if identifier == "playsound" || identifier == "playsoundbutton" { return true }
    return nodeTexts(node).contains { labels.contains(normalized($0)) }
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
    guard let row = snapshot.nodesByPath[rows[0]],
          isSemanticRow(row),
          let element = snapshot.elements[rows[0]] else {
        die("exact record \(target) is not a semantic Accessibility row")
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
    var labels = Set(options(args, "--label").map(normalized))
    labels.insert(normalized("Play Sound"))
    labels.insert(normalized("사운드 재생"))
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
    guard let row = initial.nodesByPath[rows[0]],
          isSemanticRow(row),
          let rowElement = initial.elements[rows[0]] else {
        die("exact device \(target) is not a semantic Accessibility row")
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
    guard let windowFrame = mainWindow(refreshed)?.frame else {
        die("Find My Accessibility tree has no main window")
    }
    let sidebarRight = windowFrame.x + min(480, windowFrame.width * 0.48)
    var controlPaths: [String] = []
    var seen = Set<String>()
    for node in refreshed.nodes where isPlaySoundNode(node, labels: labels) {
        guard let path = nearestPressPath(node, snapshot: refreshed),
              let control = refreshed.nodesByPath[path],
              control.role == "AXButton",
              hasAction(control, "AXPress"),
              control.enabled != false,
              let controlFrame = control.frame, controlFrame.x >= sidebarRight else { continue }
        if seen.insert(path).inserted { controlPaths.append(path) }
    }
    guard controlPaths.count == 1,
          let control = refreshed.elements[controlPaths[0]] else {
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
