#!/bin/sh
# Case-coverage check (docs/testing/README.md).
# Hard failure: a test cites a case ID that doesn't exist in docs/testing/cases.
# Report (not yet enforced while the build is young): cases without any test.
set -eu
cd "$(dirname "$0")/.."

doc_ids=$(grep -rhoE '^### [A-Z]+-[0-9]+' docs/testing/cases | sed 's/^### //' | sort -u)
test_ids=$(grep -rhoE 'Test[A-Z]+[0-9]+_' --include='*_test.go' . \
  | sed -E 's/Test([A-Z]+)([0-9]+)_.*/\1-\2/' \
  | sed -E 's/-0*([0-9])/-\1/' | sort -u)

fail=0
for id in $test_ids; do
  norm=$(echo "$doc_ids" | sed -E 's/-0*([0-9])/-\1/')
  if ! echo "$norm" | grep -qx "$id"; then
    echo "FAIL: test cites unknown case $id"
    fail=1
  fi
done

total=$(echo "$doc_ids" | wc -l)
covered=0
for id in $doc_ids; do
  flat=$(echo "$id" | tr -d '-' )
  if grep -rqE "Test${flat}_" --include='*_test.go' . 2>/dev/null; then
    covered=$((covered + 1))
  fi
done
echo "case coverage: $covered/$total cases have at least one test"
exit $fail
