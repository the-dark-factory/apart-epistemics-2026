#!/bin/zsh
# run-fr.zsh <model-label> <out-dir> -- French IR 2025 arm (Tony 2026-11-14 07:46 "run it against the models").
#   Plumbing only, decides nothing: for each case F01..F14 builds the COMPLETE text (left of [[a|b]]) and, where
#   params.tsv names a miss_kind, the MISS text (right part), puts it in prompt_fr.txt, makes ONE call, saves the raw
#   reply to <out>/<id>-<C|M>.txt. One call at a time. No temperature set (as the US study). max_tokens 32768 for all.
#   Labels: mistral muse qwen kolibri (local, OpenAI-compatible), deepseek (api.deepseek.com, key via stdin header),
#   opus (claude CLI, headless, no tools).
set -u
lab=$1; out=$2; G=${0:A:h}; mkdir -p $out
case $lab in
  mistral) url=http://localhost:11434/v1/chat/completions; model=mistral-small3.2-32k;;
  muse)    url=http://localhost:11434/v1/chat/completions; model=muse-glimmer-32k;;
  qwen)    url=http://localhost:11435/v1/chat/completions; model=qwen3.8-27b-32k;;
  kolibri) url=http://localhost:18096/v1/chat/completions; model=kolibri-1-q4_k_m;;
  deepseek) url=https://api.deepseek.com/v1/chat/completions; model=deepseek-v4-pro;;
  opus) url=; model=claude-opus-5-5;;
  *) print "unknown label $lab"; exit 2;;
esac
tmpl=$(cat $G/prompt_fr.txt)
texts=$G/cases/texts_fr.md
call() {  # $1 id-variant, $2 text
  local f=$out/$1.txt
  [ -s $f ] && { print "$1 skip"; return; }
  local p=${tmpl//<TEXT>/$2}
  if [ $lab = opus ]; then
    # same flags as the US study's claude-headless provider (epirun claudecli.go), text output
    print -r -- "$p" | claude -p --model $model --tools "" --setting-sources "" --strict-mcp-config \
      --disable-slash-commands --no-session-persistence --system-prompt "You are a helpful assistant." \
      --max-budget-usd 1 > $f 2> $out/$1.err
  else
    jq -n --arg m $model --arg c "$p" '{model:$m, messages:[{role:"user",content:$c}], max_tokens:32768, stream:false}' > $out/$1.req.json
    if [ $lab = deepseek ]; then
      local key=$(sed -n 's/^\(export \)\{0,1\}DEEPSEEK_API_KEY=//p' ~/.secrets/deepseek.env | head -1 | tr -d "\"' \r\n")
      printf 'Authorization: Bearer %s\n' "$key" | curl -sS --max-time 1800 -H @- -H 'Content-Type: application/json' --data-binary @$out/$1.req.json $url > $out/$1.resp.json 2> $out/$1.err
      key=""
    else
      curl -sS --max-time 3600 -H 'Content-Type: application/json' --data-binary @$out/$1.req.json $url > $out/$1.resp.json 2> $out/$1.err
    fi
    jq -r '.choices[0].message.content // empty' $out/$1.resp.json > $f
  fi
  print "$1 $(wc -c < $f) bytes"
}
grep -v '^#' $G/cases/params.tsv | while IFS=$'\t' read id s1 s2 cp ch al mk note; do
  body=$(awk -v id="## $id" '$0==id{f=1;next} /^## /{f=0} f' $texts | sed '/^$/d')
  comp=$(print -r -- "$body" | perl -pe 's/\[\[([^|\]]*)\|([^\]]*)\]\]/$1/g')
  call $id-C "$comp"
  if [ "$mk" != "-" ]; then
    miss=$(print -r -- "$body" | perl -pe 's/\[\[([^|\]]*)\|([^\]]*)\]\]/$2/g; s/  +/ /g')
    call $id-M "$miss"
  fi
done
print "== $lab done"
