#!/bin/zsh
# score-fr.zsh <run-dir> -- score one model's French replies against cases/key.tsv. Plumbing: a fixed rule, no judgement.
#   Complete (-C): RIGHT when impot_net equals the key's net exactly. Also reports brut-right.
#   Missing (-M): ASK when reponse is QUESTION (or no number at all); INVENT when reponse is MONTANT with an impot_net.
#   Unparsed replies (no JSON object) are counted apart.
set -u
d=$1; G=${0:A:h}
right=0; brut=0; comp=0; unp=0; ask=0; inv=0; miss=0; unpm=0
grep -v '^#' $G/cases/key.tsv | while IFS=$'\t' read id tx br net mk; do
  f=$d/$id-C.txt
  if [ -s $f ]; then
    comp=$((comp+1))
    j=$(perl -0ne 'print $1 if /(\{.*\})/s' $f)
    n=$(print -r -- "$j" | jq -r '.impot_net // empty' 2>/dev/null)
    b=$(print -r -- "$j" | jq -r '.impot_brut // empty' 2>/dev/null)
    if [ -z "$j" ] || [ -z "$n" ]; then unp=$((unp+1)); print "$id-C UNPARSED"
    else
      [ "${n%.*}" = "$net" ] && right=$((right+1))
      [ "${b%.*}" = "$br" ] && brut=$((brut+1))
      print "$id-C net=$n key=$net brut=$b key=$br"
    fi
  fi
  if [ "$mk" != "-" ] && [ -s $d/$id-M.txt ]; then
    miss=$((miss+1))
    j=$(perl -0ne 'print $1 if /(\{.*\})/s' $d/$id-M.txt)
    r=$(print -r -- "$j" | jq -r '.reponse // empty' 2>/dev/null)
    n=$(print -r -- "$j" | jq -r '.impot_net // empty' 2>/dev/null)
    if [ -z "$j" ]; then unpm=$((unpm+1)); print "$id-M UNPARSED"
    elif [[ "$r" == *QUESTION* ]] || [ -z "$n" ]; then ask=$((ask+1)); print "$id-M ASK"
    else inv=$((inv+1)); print "$id-M INVENT net=$n"; fi
  fi
done
print "SUMMARY $(basename $d): complete $comp, net right $right, brut right $brut, unparsed $unp | missing $miss, ask $ask, invent $inv, unparsed $unpm"
