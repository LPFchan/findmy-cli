# findmy-cli

Read your Find My people, device, and item locations from the macOS FindMy.app
through its Accessibility tree. Apple does not expose a public API for Find My
locations and the on-disk caches are encrypted with keychain-bound keys, so the
CLI activates FindMy.app, switches tabs, and reads the accessible sidebar and
detail text. It does not capture the screen or run OCR.

**Privacy & consent.** Location lookup is read-only and consent-bounded. It can only see the
people who have already opted in to share their location with this Mac's Apple ID
in Apple's Find My, plus your own devices and items. It returns coarse location
only (city/state, staleness, distance), bypasses no Apple access control, and
initiates no external network traffic — everything stays on-device. Use it to locate
consenting friends and family, not to monitor or track anyone without their
knowledge and consent. The separate `play-sound` command can activate only the
Play Sound control for one of your Devices and requires `--confirm`.

## Why a Go CLI plus a Swift helper

The macOS Accessibility APIs have no standard Go binding. We bundle a small
Swift binary, `findmy-helper`, that emits a normalized AX tree and exposes a
narrow allow-listed action. The Go CLI groups records, resolves names, enforces
confirmation, and formats output.

## Install

This fork is not published to Homebrew, NPM, or ClawHub. Build and install it
from `LPFchan/findmy-cli` so the installed binaries include AX parsing and the
gated Play Sound command:

```bash
git clone https://github.com/LPFchan/findmy-cli.git
cd findmy-cli
make
mkdir -p "$HOME/.local/bin"
install -m 0755 bin/findmy bin/findmy-helper "$HOME/.local/bin/"
```

Ensure `$HOME/.local/bin` is on `PATH`. For development, run `make` after source
changes and execute `bin/findmy` directly. A linked Claude Code/OpenClaw plugin
can use this checkout's `scripts/findmy.sh`, which builds the same sources.

Requirements:
- macOS (tested on 15+; FindMy.app is a Catalyst app)
- Go 1.22+
- Xcode Command Line Tools (`swiftc` and macOS frameworks)

## Usage

```
# List people in the sidebar with coarse location, staleness, distance.
findmy people
findmy people --json
findmy people --no-log

# Select a row and read the accessible detail pane (precise address).
findmy person "Omar Shahine"
findmy person "Omar Shahine" --zoom --json

# List devices in the sidebar.
findmy devices
findmy devices --json

# Read one matching device.
findmy device "Omar's iPhone"
findmy device "Omar's iPhone" --json

# Preview Play Sound. This does not activate anything.
findmy play-sound "Omar's iPhone"
findmy play-sound "Omar's iPhone" --json

# Explicitly activate Play Sound after reviewing the unique match.
findmy play-sound "Omar's iPhone" --confirm

# List items in the sidebar.
findmy items
findmy items --json

# Read one matching item.
findmy item "AirPods Pro"
findmy item "AirPods Pro" --json

# Query the SQLite history ledger populated by people/devices runs.
findmy log "Omar Shahine" --since=24h
findmy log "Omar's iPhone" --kind=devices --limit=10 --json
```

Confirmation does not reuse the dry-run tree. The helper selects the Devices
tab, resolves one exact device row from a fresh AX tree, selects it, rebuilds
the tree, verifies that the selected row or detail pane still names that exact
device, and only then presses exactly one Play Sound control. Any mismatch or
ambiguity fails closed.

Successful `findmy people`, `findmy devices`, and `findmy items` runs append parsed sidebar
records to a local SQLite ledger before printing output. Pass `--no-log` to
skip a write for one invocation. The default database path is
`$XDG_DATA_HOME/findmy-cli/history.sqlite`, or
`~/.local/share/findmy-cli/history.sqlite` when `XDG_DATA_HOME` is unset.
Set `FINDMY_HISTORY_DB=/path/to/history.sqlite` to override it.

FindMy.app menu, tab, and sidebar labels are localized on non-English macOS
installs. The CLI auto-detects the current macOS language for the supported
FindMy.app strings, and `FINDMY_LANG=fr` can be used to force a locale for
testing.

## Required macOS permissions

Grant **Accessibility** to the built or installed `findmy-helper` executable:
Settings → Privacy & Security → Accessibility. Screen Recording is not
required. Verify the exact helper binary after building:

```bash
bin/findmy-helper permissions
# {"accessibility":true}
```

After granting, relaunch the calling application. Rebuilding an unsigned
development helper can require reauthorizing that binary.

## Limitations

- The user session must be logged in and FindMy.app must expose its UI through
  Accessibility. A locked or logged-out session is not supported.
- The CLI reads sidebar and detail-pane text; map pins and coordinates are not
  extracted.
- The FindMy.app window must be openable on this Mac (you must be signed in to
  iCloud and have at least one friend sharing).
- This brings FindMy.app to the foreground and can steal focus while switching
  tabs or selecting a detail row.
- `play-sound` supports Devices only. It rejects missing and ambiguous matches,
  performs a dry run without `--confirm`, re-resolves and verifies the exact
  target in the confirmed helper process, and cannot invoke Lost Mode, erase,
  sharing, or other controls.
- Apple's TOS may consider GUI scraping out of scope. Use at your own risk.

## Layout

```
cmd/findmy/                     Go CLI
internal/findmy/                Orchestration + AX grouping/parser
helpers/findmy-helper/main.swift  AX tree + allow-listed action subcommands
bin/                            Build outputs
.claude-plugin/plugin.json      Claude Code / OpenClaw plugin manifest
commands/findmy.md              /findmy slash command
skills/findmy/SKILL.md          Auto-triggering skill
scripts/findmy.sh               Plugin wrapper (auto-builds on first use)
```

## Plugin development

The Claude Code wrapper (`scripts/findmy.sh`) builds `bin/findmy` and
`bin/findmy-helper` from this checkout on first invocation. For OpenClaw
development, build this fork and set `FINDMY_CLI_PATH` to its `bin/findmy`.
No binaries are committed.
