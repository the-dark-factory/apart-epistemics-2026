#!/bin/zsh
# gate-run.zsh <items.jsonl> <fact_gate_main> -- the proven-gate replay (Tony 10:20 "do it and run it on all 4"; 10:21
# "if its free, do all of them"). ADAPTER only (counting, not deciding): reads each return, computes four presence flags
# for the facts the 1040 calculation needs, and asks the PROVEN gate (Fact_Gate_Pkg, DF 1831, via its forged edge)
# for the verdict. P1 taxpayer DOB; P2 spouse DOB when a spouse exists; P3 every dependent's DOB; P4 every W-2's
# withholding (Box 2); P5..P8 unused (passed 1). On MISSING it prints the question the user would be asked instead of
# calling any model. Prints: <id> <set> <kind> <gate line> [question].
set -u
items=$1; gate=$2
q=( "What is your date of birth?" "What is your spouse's date of birth?" "What is the date of birth of each dependent?" "How much federal income tax was withheld (W-2 box 2) on each W-2?" )
jq -c '{id, set, kind: (.kind // "-"), input}' $items | while read -r row; do
  id=$(print -r -- $row | jq -r .id); set=$(print -r -- $row | jq -r .set); kind=$(print -r -- $row | jq -r .kind)
  f=$(print -r -- $row | jq -r .input)
  flags=$(jq -r '
    .input.return_data as $r
    | [ (if ($r.irs1040.tp_date_of_birth.value // "") == "" then 0 else 1 end),
        (if (($r.irs1040.sp_first_name.value // "") != "") and (($r.irs1040.sp_date_of_birth.value // "") == "") then 0 else 1 end),
        (if ([($r.irs1040.dependent_detail // [])[] | select((.dependent_date_of_birth.value // "") == "")] | length) > 0 then 0 else 1 end),
        (if ([($r.w2 // [])[] | select(.withholding.value == null)] | length) > 0 then 0 else 1 end) ]
    | map(tostring) | join(" ")' $f)
  out=$($gate ${=flags} 1 1 1 1)
  case $out in
    MISSING*) k=${out#MISSING }; print "$id $set $kind $out | ask: ${q[$k]}";;
    *) print "$id $set $kind $out";;
  esac
done
