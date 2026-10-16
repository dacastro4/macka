#!/usr/bin/env bash
# CC-2: every requirement ID cited in a spec heading must have at least one Go
# test whose name contains the ID without its dash (T-4 -> TestT4_...).
set -euo pipefail
cd "$(dirname "$0")/.."

ids=$(grep -rhoE '^### Requirement: .*\(([A-Z]+-[0-9]+)' openspec/specs openspec/changes 2>/dev/null \
  | grep -oE '[A-Z]+-[0-9]+' | sort -u)

missing=0
for id in $ids; do
  name="${id/-/}"
  if ! grep -rqE "func Test${name}_" --include='*_test.go' .; then
    echo "missing test for ${id} (expected func Test${name}_...)"
    missing=1
  fi
done

if [ "$missing" -ne 0 ]; then
  exit 1
fi
echo "reqcheck: $(echo "$ids" | wc -w | tr -d ' ') requirement IDs covered"
