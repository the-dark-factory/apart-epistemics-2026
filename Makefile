# Try it yourself: every step of the study, one make target each.
#
#   make            list the targets
#   make check      re-score every logged model call and compare with the published analysis (free, no keys)
#   make scoreboard one line per model: right answers, questions asked, facts invented
#   make probe      the deterministic gap check: no model, about a second
#   make prove      re-run GNATprove on every proven unit (needs GNAT and GNATprove)
#   make gate       build the proven fact gate and replay it over all 80 returns
#   make french     score every model's French replies against the proven calculator's key
#   make demo       the whole run pipeline end to end with a scripted fake model (free, no keys)
#   make try        put YOUR model through the same 80 returns and see where it lands
#
# Needs: Go 1.22+, jq, zsh. For prove and gate: GNAT, GPRbuild and GNATprove (for example via Alire).

SHELL     := /bin/zsh
REPO      := $(abspath .)
EPIRUN    := $(REPO)/harness/epirun/epirun
ITEMS     := $(REPO)/study/frozen/items.local.jsonl
PLAN      := $(REPO)/study/frozen/PLAN.md
FROZEN    := $(REPO)/study/frozen/FROZEN.txt
OUT       := $(REPO)/build
RUNS      := $(notdir $(wildcard study/runs/*))
UNITS     := $(wildcard proven/*/admit-A-*)
SEED      := 20261113

# macOS: recent SDKs report a version the bundled GNAT linker rejects; pin a target it accepts.
ifeq ($(shell uname),Darwin)
export MACOSX_DEPLOYMENT_TARGET ?= 15.0
endif

.PHONY: help check scoreboard probe prove gate french demo try clean

help:
	@sed -n '3,11p' Makefile | sed 's/^# //'

$(EPIRUN): $(wildcard harness/epirun/*.go)
	cd harness/epirun && go build -o epirun .

$(ITEMS): study/frozen/items.jsonl tools/portable-items.zsh
	@tools/portable-items.zsh

check: $(EPIRUN) $(ITEMS)
	@mkdir -p $(OUT)/analysis
	@fail=0; for r in $(RUNS); do \
	  $(EPIRUN) analyse -items $(ITEMS) -run study/runs/$$r -boot 1000 -seed $(SEED) > $(OUT)/analysis/$$r.txt 2>&1; \
	  if diff -q study/analysis/$$r.txt $(OUT)/analysis/$$r.txt > /dev/null; then echo "same as published  $$r"; \
	  else echo "DIFFERENT          $$r   (diff study/analysis/$$r.txt build/analysis/$$r.txt)"; fail=1; fi; \
	done; exit $$fail

scoreboard:
	@tools/scoreboard.zsh study/analysis/confirmatory-*.txt study/analysis/baseline-strict-*.txt

probe: $(ITEMS)
	@zsh french/scripts/TAXCALC_TY24/tools/probe/gapprobe.zsh $(ITEMS) | awk '{print $$2, $$4}' | sort | uniq -c
	@echo "expected: 40 MISS MISSING, 40 TWINS COMPLETE"

prove:
	@for u in $(UNITS); do \
	  echo "== $$u"; \
	  (cd $$u && gnatprove -P proof.gpr --level=2 --report=all -j0 > prove.out 2>&1); \
	  printf '   %s checks proved, %s not proved\n' \
	    "$$(grep -c ' proved' $$u/prove.out)" "$$(grep -cE 'medium|high|error' $$u/prove.out)"; \
	done

proven/wu-fact-gate-edge/fact_gate_main: proven/wu-fact-gate-edge/fact_gate_main.adb proven/wu-fact-gate/admit-A-20261114/fact_gate_pkg.ads
	cd proven/wu-fact-gate-edge && gprbuild -q -P edge.gpr

gate: proven/wu-fact-gate-edge/fact_gate_main $(ITEMS)
	@zsh french/scripts/TAXCALC_TY24/tools/probe/gate-run.zsh $(ITEMS) proven/wu-fact-gate-edge/fact_gate_main > $(OUT)/gate.txt
	@awk '{print $$2, $$4}' $(OUT)/gate.txt | sort | uniq -c
	@echo "expected: every MISS return stopped with a question, every TWINS return passed (full output: build/gate.txt)"

french:
	@for m in french/FRANCE_IR2025/runs/*; do \
	  FR_DIR=french/FRANCE_IR2025 zsh french/scripts/FRANCE_IR2025/score-fr.zsh $$m | grep '^SUMMARY'; \
	done

demo: $(EPIRUN) $(ITEMS)
	@rm -rf $(OUT)/demo && mkdir -p $(OUT)/demo
	@echo '[{"name":"fake-mixed","kind":"fake","fake_mode":"mixed","concurrency":4}]' > $(OUT)/demo/models.json
	@$(EPIRUN) run -items $(ITEMS) -plan $(PLAN) -frozen $(FROZEN) -models $(OUT)/demo/models.json \
	  -out $(OUT)/demo -samples 1 -seed $(SEED) -protocol MISS=turn1,TWINS=turn1 > /dev/null
	@$(EPIRUN) analyse -items $(ITEMS) -run $(OUT)/demo -boot 1000 -seed $(SEED) > $(OUT)/demo/analysis.txt
	@tools/scoreboard.zsh $(OUT)/demo/analysis.txt
	@echo "(a scripted fake model: this only shows the pipeline works. Now try a real one: make try)"

# make try BASE_URL=https://api.example.com/v1 MODEL=model-id KEY_ENV=EXAMPLE_API_KEY [PRICE_IN=… PRICE_OUT=…] [BUDGET=5] [STRICT=1]
# Any OpenAI-compatible chat endpoint. For a free local model with Ollama:
#   make try BASE_URL=http://localhost:11434/v1 MODEL=llama3.1 KEY_ENV=OLLAMA_KEY OLLAMA_KEY=none
# PRICE_IN / PRICE_OUT are US dollars per million tokens. Set your provider's real prices so that
# BUDGET (US dollars) stops the run before it can cost more; the tiny defaults suit a free local model.
BUDGET    ?= 5
PRICE_IN  ?= 0.001
PRICE_OUT ?= 0.001
TRYNAME    = $(subst /,-,$(subst :,-,$(MODEL)))$(if $(STRICT),-stronger,)

try: $(EPIRUN) $(ITEMS)
	@if [[ -z "$(BASE_URL)" || -z "$(MODEL)" || -z "$(KEY_ENV)" ]]; then \
	  echo "usage: make try BASE_URL=<…/v1> MODEL=<model id> KEY_ENV=<name of the env var holding your key> [BUDGET=5] [STRICT=1]"; exit 2; fi
	@mkdir -p $(OUT)/try-$(TRYNAME)
	@jq -n --arg u "$(BASE_URL)" --arg m "$(MODEL)" --arg k "$(KEY_ENV)" \
	  --argjson pi $(PRICE_IN) --argjson po $(PRICE_OUT) \
	  '[{name: $$m, kind: "openai", base_url: $$u, model: $$m, key_env: $$k, price_in: $$pi, price_out: $$po, concurrency: 2, max_output_tokens: 16384}]' \
	  > $(OUT)/try-$(TRYNAME)/models.json
	@echo "Running $(MODEL) on 80 returns (turn 1 only). Re-running the same command resumes where it stopped."
	@$(EPIRUN) run $(if $(STRICT),-strict-missing,) -items $(ITEMS) -plan $(PLAN) -frozen $(FROZEN) \
	  -models $(OUT)/try-$(TRYNAME)/models.json -out $(OUT)/try-$(TRYNAME) -samples 1 -seed $(SEED) \
	  -protocol MISS=turn1,TWINS=turn1 -budget-usd $(BUDGET)
	@$(EPIRUN) analyse -items $(ITEMS) -run $(OUT)/try-$(TRYNAME) -boot 1000 -seed $(SEED) > $(OUT)/try-$(TRYNAME)/analysis.txt
	@echo; echo "Your model against the study's four frontier models:"
	@tools/scoreboard.zsh $(OUT)/try-$(TRYNAME)/analysis.txt \
	  $(if $(STRICT),study/analysis/baseline-strict-*.txt,study/analysis/confirmatory-*.txt)
	@echo; echo "Full report: build/try-$(TRYNAME)/analysis.txt   Every reply: build/try-$(TRYNAME)/calls.jsonl"

clean:
	rm -rf $(OUT) $(ITEMS) harness/epirun/epirun proven/*/admit-A-*/obj proven/*/admit-A-*/prove.out \
	  proven/*-edge/obj proven/wu-fact-gate-edge/fact_gate_main proven/wu-fr-ir-edge/fr_ir_main
