#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: build-swift-helper.sh <source.swift> <output>" >&2
  exit 2
fi

source_file=$1
output_file=$2
swift_compiler=${SWIFTC:-swiftc}
legacy_map=/Library/Developer/CommandLineTools/usr/include/swift/module.modulemap
current_map=/Library/Developer/CommandLineTools/usr/include/swift/bridging.modulemap
module_cache=$(dirname "$output_file")/.swift-module-cache
swift_args=(-module-cache-path "$module_cache")

mkdir -p "$(dirname "$output_file")" "$module_cache"

# Some Apple CLT upgrades leave the pre-2024 module map beside its replacement.
# Both declare SwiftBridging, so hide only the legacy file for this compiler
# invocation. The real filesystem remains visible and is never modified.
if [[ -f "$legacy_map" && -f "$current_map" ]] &&
   grep -Eq '^[[:space:]]*module[[:space:]]+SwiftBridging([[:space:]]|\{)' "$legacy_map" &&
   grep -Eq '^[[:space:]]*module[[:space:]]+SwiftBridging([[:space:]]|\{)' "$current_map"; then
  overlay_file=$(dirname "$output_file")/.swift-vfs-overlay.yaml
  printf '%s\n' \
    '{' \
    '  "version": 0,' \
    '  "case-sensitive": "false",' \
    '  "roots": [' \
    '    {' \
    '      "type": "file",' \
    "      \"name\": \"$legacy_map\"," \
    '      "external-contents": "/dev/null"' \
    '    }' \
    '  ]' \
    '}' > "$overlay_file"
  swift_args+=(-vfsoverlay "$overlay_file")
fi

exec "$swift_compiler" "${swift_args[@]}" -O -o "$output_file" "$source_file"
