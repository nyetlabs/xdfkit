# One IMAGE-DATE.zip per corpus definition (README.md).
# Run in publish/, or with make -C publish from the root.
#
#   make            stage the zips in build/publish/stage
#   make upload     rsync -a --delete to UPLOAD_DEST (local.mk);
#                   RSYNC_FLAGS=-n for a dry run
#   make incoming   import testdata/local/incoming into the corpus JSON
#   make clean      remove build/publish
#
# CORPUS is ../ecu-corpus when that checkout exists, so git log dates the
# zips; a shallow submodule may not reach the commit. DATE skips a commit
# whose message starts chore(schema):.

ROOT := $(abspath ..)
OUT := $(ROOT)/build/publish
STAGE := $(OUT)/stage
CORPUS ?= $(or $(wildcard $(abspath $(ROOT)/../ecu-corpus)),$(ROOT)/corpus)
export CORPUS
XDFKIT := $(ROOT)/build/xdfkit

.PHONY: all stage upload clean incoming

all: stage

stage:
	@$(MAKE) --no-print-directory -C $(ROOT) build
	@mkdir -p $(STAGE) $(OUT)/work
	@set -eu -o pipefail; keep=$(OUT)/keep; : > $$keep; \
	php index.php $$(awk -F '\t' 'NR > 1 && $$10 != "-" { print $$1 }' $(CORPUS)/corpus.tsv); \
	awk -F '\t' 'NR > 1 && $$10 != "-" { print $$1, $$4, $$10 }' $(CORPUS)/corpus.tsv | \
	while read -r name family def; do \
		src=$(CORPUS)/$$def; img=$(CORPUS)/images/$$name.bin; \
		d=$$(git -C $(CORPUS) log -1 --format=%cd --date=format:%Y%m%d --invert-grep --grep='^chore(schema):' -- $$def); \
		test -n "$$d"; tuner=; \
		case $$family in ME7*) tuner=1; \
			c=$$(git -C $(CORPUS) log -1 --format=%cd --date=format:%Y%m%d --invert-grep --grep='^chore(schema):' -- categories.json); \
			test -n "$$c"; if test "$$c" \> "$$d"; then d=$$c; fi ;; esac; \
		zip=$(STAGE)/$$name-$$d.zip; echo $$zip >> $$keep; \
		if test -f $$zip && test $$zip -nt $$src && test $$zip -nt $$img && test $$zip -nt $(XDFKIT) \
			&& { test -z "$$tuner" || test $$zip -nt $(CORPUS)/categories.json; }; then continue; fi; \
		w=$(OUT)/work/$$name; rm -rf $$w; mkdir -p $$w; cp -p $$src $$w/$$name.json; \
		st=$$($(XDFKIT) verify --status $$w/$$name.json) || exit 1; \
		q=; \
		if test "$$st" = edited; then q=--quiet; fi; \
		$(XDFKIT) $$q --force $$w/$$name.json $$w/$$name.kp; \
		$(XDFKIT) $$q -f csv --force -i $$img $$w/$$name.json $$w/$$name.csv; \
		$(XDFKIT) $$q -f xdf --force -i $$img $$w/$$name.json $$w/$$name.xdf; \
		set -- $$name.json $$name.kp $$name.csv $$name.xdf $$name.meta.json; \
		if test -n "$$tuner"; then \
			$(XDFKIT) $$q -f csv --force --tuner $(CORPUS)/categories.json -i $$img $$w/$$name.json $$w/$$name-tuner.csv; \
			$(XDFKIT) $$q -f xdf --force --tuner $(CORPUS)/categories.json -i $$img $$w/$$name.json $$w/$$name-tuner.xdf; \
			set -- "$$@" $$name-tuner.csv $$name-tuner.xdf $$name-tuner.meta.json; \
		fi; \
		(cd $$w && zip -qX $$zip "$$@"); \
	done; \
	for z in $(STAGE)/*.zip; do test -e $$z || continue; grep -qxF "$$z" $$keep || rm "$$z"; done; \
	rm -rf $(STAGE)/full; \
	find $(STAGE) -maxdepth 1 -type f ! -name '*.zip' -delete; \
	cp index.php $(STAGE)/

upload: stage
	$(if $(UPLOAD_DEST),,$(error set UPLOAD_DEST in local.mk))
	@rsync -a --delete $(RSYNC_FLAGS) $(STAGE)/ $(UPLOAD_DEST)

incoming:
	@./import-incoming.sh

clean:
	@rm -rf $(OUT)

-include local.mk
