---
name: findmy
description: |
  Query Find My friend locations on macOS via the findmy-cli plugin tools.
  Returns name, coarse location (city, state), staleness, and distance for
  everyone in the FindMy.app People sidebar. Use when the user asks "where
  is X", "is X home", "how far is X", or wants a location refresh.
license: MIT
metadata:
  author: Omar Shahine
  version: 0.3.2
  openclaw:
    emoji: pushpin
    os: [darwin]
    homepage: https://github.com/LPFchan/findmy-cli
    requires:
      bins: [findmy, findmy-helper]
---

# Find My Skill

Two tools available, both shell out to the `findmy` binary, which reads
FindMy.app through macOS Accessibility. These plugin tools are read-only.

## Privacy & consent (read first)

- These tools can only read people who have **already opted in** to share their
  location with this Mac's Apple ID in Apple's Find My. Sharing is mutual and the
  other person controls it — the plugin cannot see anyone who has not consented,
  and does not bypass any Apple access control.
- Returns coarse location only (city/state, staleness, distance), the same data
  FindMy.app shows the signed-in user. No precise coordinates, history, or
  background tracking.
- Intended for the Mac's owner to locate consenting friends and family. **Not a
  surveillance tool** — never use it to monitor or track someone without their
  knowledge and consent.
- When a result is `Paused` or stale, surface that plainly; the position may be
  hours or days old, and reporting it as current is misleading.

## When to use

- "Where is Omar?"        → call `findmy_person` with `name: "Omar"`
- "Is Sarah home yet?"    → call `findmy_person` with `name: "Sarah"`
- "How far away is Mike?" → call `findmy_person` with `name: "Mike"`
- "Anyone near downtown?" → call `findmy_people`, then read the locations
- "Where is everyone?"    → call `findmy_people`
- "Where am I?"           → call `findmy_person` with `name: "Me"`. The first
                            entry in the FindMy People sidebar is always the
                            owner of this Mac, labeled `"Me"`. Same shape as
                            other entries but with no `staleness` or `distance`.

## Using location to drive other actions

Location is high-trust data — knowing where someone is can unlock useful
follow-ups: "you're near Pike Place, want me to book a table at Matt's?",
"Sarah's still 7 mi away, push the dinner reservation by 30 min", "you're
home, run the arrival routine."

Before chaining a location result into a mutating action (booking, sending
a message, triggering a routine, ordering something), **ask the user for
explicit approval first**. State the location you used, the action you'd
take, and wait for confirmation. Examples:

- ✅ "You're in Seattle near downtown. Want me to search OpenTable for
  dinner reservations within 1 mile?"
- ✅ "Sarah is 15 min away. Should I text her the parking instructions?"
- ❌ Calling `restaurant_book` based purely on the inferred location, no
  confirmation step.

Read-only location queries themselves are what the account owner asked for and
are consent-bounded (only people already sharing show up). The approval gate is
on the *next* tool call that turns location into an action.

## Output shape

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
- `distance` — distance from this Mac if FindMy shows it (e.g. `"7 mi"`)

## Caveats to surface to the user

- **`staleness: "Paused"`** means the friend paused location sharing. The
  reported location is the last known position, possibly hours or days old.
  Lead with this when reporting the result.
- **Stale staleness** (`"7 hr. ago"`, etc.) means the device hasn't checked
  in recently — phone may be off, in low-power mode, or out of signal.
- **Focus steal**: each invocation briefly raises FindMy.app to the front.
- The CLI also has a device-only `play-sound` command. It is not exposed as an
  OpenClaw tool and is a dry run unless a human explicitly supplies
  `--confirm`. Confirmed actions freshly resolve and verify the exact selected
  device inside one helper process.

## Install requirement

The plugin shells out to this fork's `findmy` binary. If a tool returns
`"findmy not found on PATH"`, build the fork and configure its path:

```bash
git clone https://github.com/LPFchan/findmy-cli.git
cd findmy-cli
make
export FINDMY_CLI_PATH="$PWD/bin/findmy"
```

After building, grant **Accessibility** to this checkout's
`bin/findmy-helper` executable (System Settings → Privacy & Security →
Accessibility).

## Implementation note

This skill raises FindMy.app, switches to the People tab, and parses accessible
sidebar text. It does not capture screenshots or run OCR. The plugin tools do
not expose the CLI's separate Play Sound action.
