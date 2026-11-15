#!/bin/zsh
# portable-items.zsh — make a copy of the frozen item list that works on your machine.
#
# The frozen study/frozen/items.jsonl records the absolute paths of the machine that ran the
# study. It is left untouched (its sha256 is in study/frozen/FROZEN.txt, so you can check it).
# This writes study/frozen/items.local.jsonl with those paths pointing into this repository.
#
# Usage: tools/portable-items.zsh          (run from anywhere; needs jq)
set -e
repo=${0:A:h:h}
old=/Users/tony/dev/bustle-memory-20261027/guides/TAXCALC_TY24/fresh
new=$repo/study/frozen/fresh
jq -c --arg o "$old" --arg n "$new" '
  .input |= sub("^" + $o; $n)
  | if .key_file then .key_file |= sub("^" + $o; $n) else . end
' $repo/study/frozen/items.jsonl > $repo/study/frozen/items.local.jsonl
missing=0
for f in $(jq -r '.input, (.key_file // empty)' $repo/study/frozen/items.local.jsonl); do
  [[ -f $f ]] || { echo "missing: $f" >&2; missing=$((missing + 1)); }
done
if (( missing > 0 )); then
  echo "portable-items: $missing file(s) not found" >&2
  exit 1
fi
echo "wrote study/frozen/items.local.jsonl ($(wc -l < $repo/study/frozen/items.local.jsonl | tr -d ' ') items, every path checked)"
