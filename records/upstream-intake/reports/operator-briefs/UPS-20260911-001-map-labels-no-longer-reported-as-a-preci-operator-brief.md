# UPS-20260911-001 — operator verdict: accept

## Review Metadata

- Review id: UPS-20260911-001
- Opened: `2026-09-11 01-00-55 KST`
- Recorded by agent: upstream-dashboard
- Fork: LPFchan/findmy-cli
- Upstream window: `29a2df7fc7..1206b058bc`
- Decision: accept
- Decision owner: operator (via upstream dashboard)

## Candidate Change

- Title: Map labels no longer reported as a precise address
- Upstream summary: Fixes user issue #13: when zooming into a device location, nearby map labels (point-of-interest names) were being presented as the device's precise address.
- Upstream commits: 829b3ef1fc

## Intake Analysis (dashboard-generated)

Before: The CLI could print a landmark or business label as if it were the exact address where the device was found.
After: The reported address reflects the actual resolved location; map labels are no longer promoted to addresses.
Concrete consequence: Location output becomes trustworthy; users stop being sent to places the device never entered.
Literal scenario: A user locates their phone parked near a cafe; previously the CLI printed the cafe's name and street address as the precise location and they drove there. After the fix, the same query reports the real coordinates and address.

Direct correctness fix in shared location-reporting logic. The fork has no reason to preserve the old misleading behavior; low merge risk.

## Decision Rationale

- Recommendation was: accept
- Operator verdict: accept
