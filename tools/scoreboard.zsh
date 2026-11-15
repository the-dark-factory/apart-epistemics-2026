#!/bin/zsh
# scoreboard.zsh — one line per model, from epirun analyse reports.
#
# For each report: right answers on the 40 complete returns, unneeded questions on those
# complete returns, and on the 40 missing-fact returns how often the model ASKED and how often
# it INVENTED, split by the missing fact (a child's date of birth, or the tax withheld), and how
# often it GAVE UP on them (CANNOT, an unreadable reply, or one cut off), so that a model which
# barely answers cannot look as if it never invents.
#
# Usage: tools/scoreboard.zsh report.txt [report.txt ...]
(( $# > 0 )) || { echo "usage: $0 report.txt [...]" >&2; exit 2; }
printf '%-34s %-18s %-8s %-9s %-10s %-13s %-9s %-12s %-8s\n' run model right unneeded "DOB asked" "DOB invented" "WH asked" "WH invented" "gave up"
for r in "$@"; do
  awk '
    /^run / { run = $2; sub(":", "", run) }
    /^1\. WHOLE-RETURN/ { s = 1; next }
    /^2\. ASK, INVENT/  { s = 2; next }
    /^3\./              { s = 0 }
    $1 == "model" || $1 == "PANEL" || NF < 9 { next }
    s == 1 { name = $1; right[name] = $11; unneeded[name] = $7 "/" $3; order[++n] = name }
    s == 2 && $2 == "all" { gu[$1] = $15 }
    s == 2 && $2 == "DOB" { da[$1] = $6; di[$1] = $9 }
    s == 2 && $2 == "WH"  { wa[$1] = $6; wi[$1] = $9 }
    END {
      for (i = 1; i <= n; i++) {
        m = order[i]
        printf "%-34s %-18s %-8s %-9s %-10s %-13s %-9s %-12s %-8s\n", run, m, right[m], unneeded[m], da[m], di[m], wa[m], wi[m], gu[m]
      }
    }
  ' "$r"
done
