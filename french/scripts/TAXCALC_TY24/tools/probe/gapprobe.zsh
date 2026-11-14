#!/bin/zsh
# gapprobe.zsh <items.jsonl> -- the deterministic gap probe (SKETCH_unplanned_bims_probe_first_2026-11-14, first step;
# Tony 09:53 "Yes, reframe it"). HAND-BUILT, NOT PROVEN, uses no model. For every item it checks the taxpayer data
# against a FIXED list of facts the Form 1040 calculation needs, written from the form, not from how the twins were made:
#   tp_date_of_birth; sp_date_of_birth when a spouse is present (sp_first_name given);
#   dependent_date_of_birth for every dependent; withholding (W-2 Box 2) for every W-2.
# Prints: <id> <set> <kind> COMPLETE | MISSING <field,...>. Reads files only; decides nothing about tax.
set -u
items=$1
jq -c '{id, set, kind: (.kind // "-"), input}' $items | while read -r row; do
  id=$(print -r -- $row | jq -r .id); set=$(print -r -- $row | jq -r .set); kind=$(print -r -- $row | jq -r .kind)
  f=$(print -r -- $row | jq -r .input)
  miss=$(jq -r '
    .input.return_data as $r
    | [ (if ($r.irs1040.tp_date_of_birth.value // "") == "" then "tp_date_of_birth" else empty end),
        (if (($r.irs1040.sp_first_name.value // "") != "") and (($r.irs1040.sp_date_of_birth.value // "") == "")
           then "sp_date_of_birth" else empty end),
        ( ($r.irs1040.dependent_detail // []) | to_entries[]
          | select((.value.dependent_date_of_birth.value // "") == "") | "dependent_date_of_birth[\(.key)]" ),
        ( ($r.w2 // []) | to_entries[]
          | select(.value.withholding.value == null) | "w2[\(.key)].withholding" ) ]
    | join(",")' $f)
  if [ -z "$miss" ]; then print "$id $set $kind COMPLETE"; else print "$id $set $kind MISSING $miss"; fi
done
