#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   ./scripts/check-govulncheck.sh [govulncheck-json-file]

INPUT_FILE="${1:-govulncheck.json}"

# False positives:
# https://github.com/golang/vulndb/issues/4943
# https://github.com/golang/vulndb/issues/4944
IGNORED_CVES=(
  "CVE-2026-33815"
  "CVE-2026-33816"
)

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required but was not found in PATH." >&2
  exit 2
fi

if [ ! -f "$INPUT_FILE" ]; then
  echo "Input file not found: $INPUT_FILE" >&2
  exit 2
fi

IGNORED_JSON="$(printf '%s\n' "${IGNORED_CVES[@]}" | jq -Rn '[inputs]')"

result="$(jq -s --argjson ignored "$IGNORED_JSON" '
  [ .[] | select(.osv != null) | {id: .osv.id, aliases: (.osv.aliases // [])} ] as $osvs
  | [ .[] | select(.finding != null) | .finding.osv ] | unique | map(
      . as $fid
      | ($osvs[] | select(.id == $fid)) as $meta
      | {
          osv: $fid,
          aliases: ($meta.aliases // []),
          ignored: (($meta.aliases // []) | any(IN($ignored[])))
        }
    )
  | {
      ignored: map(select(.ignored)),
      actionable: map(select(.ignored | not))
    }
' "$INPUT_FILE")"

ignored_count="$(echo "$result" | jq '.ignored | length')"
actionable_count="$(echo "$result" | jq '.actionable | length')"

echo "Ignored findings: ${ignored_count}"
if [ "$ignored_count" -gt 0 ]; then
  echo "$result" | jq -r '.ignored[] | "  \(.osv) [\(.aliases | join(", "))]"'
fi

if [ "$actionable_count" -gt 0 ]; then
  echo "Actionable vulnerabilities: ${actionable_count}"
  echo "$result" | jq -r '.actionable[] | "  \(.osv) [\(.aliases | join(", "))]"'
  exit 1
fi

echo "No actionable vulnerabilities."
