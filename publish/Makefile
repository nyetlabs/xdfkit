# Generate the published map definitions, formerly ecuxplot's data/Makefile.
# Outputs go to build/publish/, the published tree to build/publish/stage/.
# Run in publish/, or with make -C publish from the root.
#
#   make                  every pack's files (make all)
#   make help             this help
#   make zips             one dated zip per pack and location
#   make stage            the published tree, in build/publish/stage
#   make upload           rsync --delete the tree to UPLOAD_DEST (set it in
#                         local.mk, gitignored); RSYNC_FLAGS=-n for a dry run
#   make incoming         import every KP and XDF in testdata/local/incoming
#                         into the corpus JSON (import-incoming.sh, README.md)
#   make clean            remove build/publish
#
# Each pack is the model JSON in the corpus, defs/IMAGE.json, with IMAGE from
# the archive's images.tsv; KP, CSV and XDF are written from it by xdfkit (KP
# without a template). A pack without corpus JSON is converted from its
# archived KP (testdata/archive/ecuxplot): 8N0906018CB, whose pack was made for
# a non-OEM image kept in the archive until an OEM image of that software is
# in the corpus. CORPUS is the ecu-corpus checkout: ../ecu-corpus beside this
# repo when it exists (the full clone corpus edits are committed in, so zips
# get their JSON's real date), else the shallow submodule.
#
# Layout (README.md): a DAMOS pack (origin damos) is published in full/; an
# ME7 one also gets a tuner subset at the top level, PACK-tuner (XDF, metadata
# file and CSV: only the maps in CATEGORIES, the corpus categories.json, and
# their axes). Every other pack is a hand pack, at the top level. Each
# location holds the files and one zip dated by the last commit of the pack's
# source (and of categories.json for a tuner zip); older zips are not kept.

ROOT := $(abspath ..)
OUT := $(ROOT)/build/publish
STAGE := $(OUT)/stage
ARCHIVE := $(ROOT)/testdata/archive/ecuxplot
CORPUS ?= $(or $(wildcard $(abspath $(ROOT)/../ecu-corpus)),$(ROOT)/corpus)
export CORPUS
CATEGORIES ?= $(CORPUS)/categories.json
XDFKIT := $(ROOT)/build/xdfkit

REFS := 4Z7907551R 8D0907551F 8D0907551G
PACKS := $(REFS) 8D0907551H 8D0907551K 8D0907551M 4Z7907551AA 4D1907558 8D0907558E 8D0907558M 06A906032HS 06A906032LP 8N0906018CB

REFS_8D0907551H := $(addprefix -r ,$(addsuffix .json,$(REFS)))
REFS_8D0907551M := $(REFS_8D0907551H)

# For packs without corpus JSON or corpus image.
ORIGIN_8N0906018CB := damos
FAMILY_8N0906018CB := ME7.5

imagename = $(shell awk -F'\t' '$$1 == "$(1)" { print $$2 }' $(ARCHIVE)/images.tsv)
image = $(or $(wildcard $(ARCHIVE)/$(1).bin),$(CORPUS)/images/$(call imagename,$(1)).bin)
def = $(CORPUS)/defs/$(call imagename,$(1)).json
source = $(or $(wildcard $(call def,$(1))),$(ARCHIVE)/$(1).kp)
packorigin = $(or $(ORIGIN_$(1)),$(if $(wildcard $(call def,$(1))),$(shell jq -r '.provenance.origin // empty' $(call def,$(1)))))
family = $(or $(FAMILY_$(1)),$(shell awk -F'\t' '$$1 == "$(call imagename,$(1))" { print $$4 }' $(CORPUS)/corpus.tsv))

# date PACK[,tuner]: the last commit of the pack's source, and of
# categories.json for a tuner zip.
log = git -C $(1) log -1 --format=%cd --date=format:%Y%m%d -- $(2);
date = $(shell { $(if $(wildcard $(call def,$(1))),$(call log,$(CORPUS),defs/$(call imagename,$(1)).json),$(call log,$(ROOT),$(ARCHIVE)/$(1).kp)) $(if $(2),$(call log,$(CORPUS),categories.json)) } | sort | tail -1)

FULL := $(strip $(foreach p,$(PACKS),$(if $(filter damos,$(call packorigin,$(p))),$(p))))
TUNER := $(if $(wildcard $(CATEGORIES)),$(strip $(foreach p,$(FULL),$(if $(filter ME7%,$(call family,$(p))),$(p)))))
HAND := $(filter-out $(FULL),$(PACKS))

EXTS := .kp .xdf .meta.json .csv .json
TUNER_EXTS := .xdf .meta.json .csv

# files STEM,EXTS,DATE: the files and the dated zip of one location
files = $(addprefix $(OUT)/$(1),$(2)) $(OUT)/$(1)-$(3).zip
TOP := $(foreach p,$(HAND),$(call files,$(p),$(EXTS),$(call date,$(p)))) \
	$(foreach p,$(TUNER),$(call files,$(p)-tuner,$(TUNER_EXTS),$(call date,$(p),tuner)))
FULLDIR := $(foreach p,$(FULL),$(call files,$(p),$(EXTS),$(call date,$(p))))
ZIPS := $(filter %.zip,$(TOP) $(FULLDIR))

.PHONY: all help zips stage upload clean incoming

all: $(filter-out %.zip,$(TOP) $(FULLDIR))
	@echo "files up to date in $(OUT)"

help:
	@sed -n '/^#   make/,/^#$$/p' Makefile | sed '$$d; s/^# //'
	@echo
	@echo 'hand: $(HAND)'
	@echo 'full: $(FULL)'
	@echo 'tuner: $(or $(TUNER),none ($(CATEGORIES) missing?))'

clean:
	rm -rf $(OUT)

$(OUT):
	mkdir -p $@

$(XDFKIT):
	$(MAKE) -C $(ROOT) build

.SECONDEXPANSION:
$(OUT)/%.json: $$(call source,$$*) | $(OUT) $(XDFKIT)
	$(XDFKIT) -force $(if $(ORIGIN_$*),-origin $(ORIGIN_$*)) $< $@

$(OUT)/%.kp: $(OUT)/%.json | $(XDFKIT)
	$(XDFKIT) -force $< $@

# The tuner rules come before the full ones, which make 3.81 would pick first
# for PACK-tuner.*. One XDF run writes both files.
$(OUT)/%-tuner.csv: $(OUT)/%.json $(CATEGORIES) | $(XDFKIT)
	$(XDFKIT) -f csv -force -tuner $(CATEGORIES) -i $(call image,$*) $< $@

$(OUT)/%-tuner.xdf $(OUT)/%-tuner.meta.json: $(OUT)/%.json $(CATEGORIES) | $(XDFKIT)
	$(XDFKIT) -f xdf -force -tuner $(CATEGORIES) -i $(call image,$*) $< $(OUT)/$*-tuner.xdf

$(OUT)/8D0907551H.csv $(OUT)/8D0907551M.csv: $(addprefix $(OUT)/,$(addsuffix .json,$(REFS)))

$(OUT)/%.csv: $(OUT)/%.json | $(XDFKIT)
	cd $(OUT) && $(XDFKIT) -f csv -force -i $(call image,$*) $(REFS_$*) $*.json $*.csv

$(OUT)/%.xdf $(OUT)/%.meta.json: $(OUT)/%.json | $(XDFKIT)
	$(XDFKIT) -f xdf -force -i $(call image,$*) $< $(OUT)/$*.xdf

# ZIP_template STEM,EXTS,DATE: the zip holds the files renamed STEM-DATE.EXT,
# so the XDF and its metadata file keep one stem.
define ZIP_template
$(OUT)/$(1)-$(3).zip: $(addprefix $(OUT)/$(1),$(2))
	cd $(OUT) && rm -f $$@ && for e in $(2); do cp -p $(1)$$$$e $(1)-$(3)$$$$e; done && \
		zip -qX $$@ $(addprefix $(1)-$(3),$(2)) && rm $(addprefix $(1)-$(3),$(2))
endef

$(foreach p,$(HAND) $(FULL),$(eval $(call ZIP_template,$(p),$(EXTS),$(call date,$(p)))))
$(foreach p,$(TUNER),$(eval $(call ZIP_template,$(p)-tuner,$(TUNER_EXTS),$(call date,$(p),tuner))))

zips: $(ZIPS)

stage: $(TOP) $(FULLDIR)
	rm -rf $(STAGE) && mkdir -p $(STAGE)/full
	cp -p $(TOP) $(STAGE)/
	$(if $(FULLDIR),cp -p $(FULLDIR) $(STAGE)/full/)

upload: stage
	$(if $(UPLOAD_DEST),,$(error set UPLOAD_DEST in local.mk))
	rsync -av --delete $(RSYNC_FLAGS) $(STAGE)/ $(UPLOAD_DEST)

incoming:
	./import-incoming.sh

-include local.mk

.SECONDARY: $(addprefix $(OUT)/,$(addsuffix .json,$(PACKS)))
.DELETE_ON_ERROR:
