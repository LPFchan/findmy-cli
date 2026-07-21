---
name: findmy
description: |
  Query Find My friend locations on macOS. Returns name, coarse location,
  staleness, and distance for everyone in the FindMy.app People sidebar.
  Use when the user asks "where is X", "is X home", "how far is X", or
  wants a location refresh for a friend. macOS only; requires the
  Accessibility permission granted to the bundled `findmy-helper` binary.
---

# Find My Location Query

Wraps the `findmy` CLI bundled with this plugin. The CLI activates FindMy.app,
switches to the People tab, and parses its Accessibility tree. It does not
capture the screen or run OCR.

## When to use

- "Where is Omar?"
- "Is Sarah home yet?"
- "How far away is Mike?"
- "Anyone near downtown?"

## Run

```bash
# List everyone in the sidebar (default: human-readable table)
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" people

# Same, as JSON — best for programmatic follow-up
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" people --json

# Lookup a single friend (name match is case-insensitive, substring OK)
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" person "Omar Shahine" --json
```

The wrapper auto-builds this checkout's `bin/findmy` and `bin/findmy-helper` on
first run via `make` (needs Go 1.22+ and Xcode Command Line Tools with
`swiftc`). Subsequent runs execute those fork binaries.

## Output shape (JSON)

```json
[
  {
    "name": "Omar Shahine",
    "location": "Redmond, WA",
    "staleness": "Paused",
    "distance": "7 mi"
  }
]
```

- `name` — display name as shown in the FindMy sidebar
- `location` — city, state (or device label when sharing from a device)
- `staleness` — `"Now"`, `"X min. ago"`, `"X hr. ago"`, `"Paused"`, `""` (live)
- `distance` — distance from this Mac if FindMy shows it (e.g. `"7 mi"`, `"1,971 mi"`)

## Caveats — surface these when relevant

- **`staleness: "Paused"`** means the friend has paused location sharing. The
  reported location is the last known position, possibly hours or days old.
  Lead with this when reporting the result.
- **Focus steal**: each invocation briefly raises FindMy.app to the front.
- **No coordinates**: this reads accessible sidebar text; lat/lon is not
  available. Apple doesn't expose friend locations through any public API.
- **Play Sound is separate**: location tools remain read-only. The CLI's
  device-only `play-sound` command is a dry run unless the user explicitly
  supplies `--confirm`; do not add that flag without direct user approval. A
  confirmed invocation freshly resolves and verifies the exact selected device
  before activating the single allow-listed control.

## Permission requirements (one-time)

- **Accessibility** — grant the built `bin/findmy-helper` executable in System
  Settings → Privacy & Security → Accessibility.

After granting, relaunch the calling application. The helper's `permissions`
subcommand verifies the exact binary:

```bash
"${CLAUDE_PLUGIN_ROOT}/bin/findmy-helper" permissions
# → {"accessibility":true}
```
