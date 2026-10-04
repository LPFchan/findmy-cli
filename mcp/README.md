# Persistent Find My MCP

This optional server wraps the installed fork CLI with the official Go MCP SDK.
It answers requests on demand; it does not poll or ring devices. The seven tools
are `findmy_people`, `findmy_person`, `findmy_devices`, `findmy_device`,
`findmy_items`, `findmy_item`, and `findmy_status`. Sidebar list requests use
`--no-log`. Individual lookups accept `name` and optional `zoom`; item zoom is
not supported by the CLI. Status only checks Accessibility permission.

Requests that use Find My are serialized because its UI is shared. Each request
has a 45-second deadline including queue time. Cancellation kills the CLI and
its Swift helper together before another request uses the UI. Queries can raise
Find My and select tabs or rows. A logged-in, unlocked macOS session, iCloud
sign-in, and Accessibility permission for the installed helper are required.
The server remains available when Accessibility is missing; status reports the
missing grant and location tools return an error.

## Build and test

The server is a separate Go module requiring Go 1.25+, so the CLI keeps its
existing Go requirement. Build the CLI/helper with `make` from the repo root.

```sh
cd mcp
go test -race ./...
go vet ./...
go build -o ../bin/findmy-mcp .
```

Install `findmy`, `findmy-helper`, and `findmy-mcp` into `~/.local/bin`.
Grant Accessibility to that exact `findmy-helper` in macOS System Settings.
Rebuilding an unsigned helper can require reauthorization.

## Dumpling deployment

The server listens at `127.0.0.1:8786`. Tailscale Serve proxies it over private
HTTPS at `https://dumpling.tailaa113.ts.net:8443/mcp`; it is not a public Funnel.
Every request requires `Authorization: Bearer <token>`, including `/healthz`.
The token is saved in Passage as `mcp/FINDMY_DUMPLING_TOKEN`; its runtime copy is
`~/.config/findmy-mcp/token` with mode `0600`. Never commit its value.
Only localhost and the configured proxy Host are accepted; cross-site browser
requests are rejected. The service logs startup/failure messages, not queries,
locations, or bearer tokens.

The checked-in plist starts in the user's Aqua session and restarts on exit.
Render its `__HOME__` placeholders before loading it:

```sh
mkdir -p ~/Library/LaunchAgents ~/Library/Logs
sed "s|__HOME__|$HOME|g" com.lost.plus.findmy-mcp.plist \
  > ~/Library/LaunchAgents/com.lost.plus.findmy-mcp.plist
plutil -lint ~/Library/LaunchAgents/com.lost.plus.findmy-mcp.plist
launchctl bootstrap "gui/$(id -u)" \
  ~/Library/LaunchAgents/com.lost.plus.findmy-mcp.plist
tailscale serve --bg --https=8443 --yes http://127.0.0.1:8786
```

Clients should use Streamable HTTP with the private HTTPS URL and bearer header.
Retrieve the token from Passage into the client's secret configuration; avoid
putting its literal value in command arguments or version-controlled files.
`findmy_status` is a safe connectivity check that reads no locations and rings
nothing. No agent client configuration is changed by this deployment.

To inspect or restart:

```sh
launchctl print "gui/$(id -u)/com.lost.plus.findmy-mcp"
launchctl kickstart -k "gui/$(id -u)/com.lost.plus.findmy-mcp"
tail -n 40 ~/Library/Logs/findmy-mcp.log
```

To stop/remove the service, unload the LaunchAgent and disable only its Serve
listener (do not reset other Tailscale services):

```sh
launchctl bootout "gui/$(id -u)/com.lost.plus.findmy-mcp"
tailscale serve --https=8443 off
rm ~/Library/LaunchAgents/com.lost.plus.findmy-mcp.plist
```
