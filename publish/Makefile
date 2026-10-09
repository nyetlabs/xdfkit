# Generate the published map definitions, formerly ecuxplot's data/Makefile:
# per pack a KP, a CSV map list and an XDF with its metadata file
# (NAME.meta.json, docs/stamp-and-metadata.md), and dated zips. Outputs go to
# build/publish/. Run in publish/, or with make -C publish
# from the root.
#
#   make                  KP, CSV, XDF and metadata file for every pack (make all)
#   make help             this help
#   make zips             dated and -latest zips
#   make upload           publish, if local.mk (gitignored) defines it
#   make incoming         import every KP and XDF in testdata/local/incoming
#                         into the corpus JSON (import-incoming.sh, README.md)
#   make clean            remove build/publish
#
# Each pack is the model JSON in the corpus, defs/IMAGE.json, with IMAGE from
# the archive's images.tsv; KP, CSV and XDF are written from it by xdfkit (KP
# without a template). A pack without corpus JSON is converted from its
# archived KP (testdata/archive/ecuxplot): 8N0906018CB, whose pack was made for
# a non-OEM image kept in the archive until an OEM image of that software is
# in the corpus. CORPUS can point at an ecu-corpus checkout with JSON that the
# submodule doesn't have yet.

ROOT := $(abspath ..)
OUT := $(ROOT)/build/publish
ARCHIVE := $(ROOT)/testdata/archive/ecuxplot
CORPUS ?= $(ROOT)/corpus
XDFKIT := $(ROOT)/build/xdfkit

REFS := 4Z7907551R 8D0907551F 8D0907551G
PACKS := $(REFS) 8D0907551H 8D0907551K 8D0907551M 4Z7907551AA 4D1907558 8D0907558E 8D0907558M 06A906032HS 06A906032LP 8N0906018CB
ZIP_BINS := 8D0907551M 4D1907558

REFS_8D0907551H := $(addprefix -r ,$(addsuffix .json,$(REFS)))
REFS_8D0907551M := $(REFS_8D0907551H)

imagename = $(shell awk -F'\t' '$$1 == "$(1)" { print $$2 }' $(ARCHIVE)/images.tsv)
image = $(or $(wildcard $(ARCHIVE)/$(1).bin),$(CORPUS)/images/$(call imagename,$(1)).bin)
def = $(CORPUS)/defs/$(call imagename,$(1)).json
source = $(or $(wildcard $(call def,$(1))),$(ARCHIVE)/$(1).kp)

# A zip is dated by its pack JSON's last corpus commit.
date = $(shell git -C $(CORPUS) log -1 --format=%cd --date=format:%Y%m%d -- defs/$(call imagename,$(1)).json)

EXTS := kp xdf meta.json csv
TARGETS := $(foreach p,$(PACKS),$(addprefix $(OUT)/$(p).,$(EXTS)))

.PHONY: all help zips clean incoming

all: $(TARGETS)
	@echo "$(words $(TARGETS)) files up to date in $(OUT)"

help:
	@sed -n '/^#   make/,/^#$$/p' Makefile | sed '$$d; s/^# //'
	@echo
	@echo 'packs: $(PACKS)'
	@echo 'zipped: $(ZIP_BINS)'

clean:
	rm -rf $(OUT)

$(OUT):
	mkdir -p $@

$(XDFKIT):
	$(MAKE) -C $(ROOT) build

.SECONDEXPANSION:
$(OUT)/%.json: $$(call source,$$*) | $(OUT) $(XDFKIT)
	$(XDFKIT) -force $< $@

$(OUT)/%.kp: $(OUT)/%.json | $(XDFKIT)
	$(XDFKIT) -force $< $@

$(OUT)/8D0907551H.csv $(OUT)/8D0907551M.csv: $(addprefix $(OUT)/,$(addsuffix .json,$(REFS)))

$(OUT)/%.csv: $(OUT)/%.json | $(XDFKIT)
	cd $(OUT) && $(XDFKIT) -f csv -force -i $(call image,$*) $(REFS_$*) $*.json $*.csv

# one run writes both
$(OUT)/%.xdf $(OUT)/%.meta.json: $(OUT)/%.json | $(XDFKIT)
	$(XDFKIT) -f xdf -force -i $(call image,$*) $< $(OUT)/$*.xdf

define ZIP_template
$(OUT)/$(1)-$(call date,$(1)).zip: $(addprefix $(OUT)/$(1).,$(EXTS))
	cd $(OUT) && rm -f $$@ && for e in $(EXTS); do cp $(1).$$$$e $(1)-$(call date,$(1)).$$$$e; done && \
		zip $$@ $(addprefix $(1)-$(call date,$(1)).,$(EXTS)) && rm $(addprefix $(1)-$(call date,$(1)).,$(EXTS))
$(OUT)/$(1)-latest.zip: $(OUT)/$(1)-$(call date,$(1)).zip
	ln -sf $$(notdir $$<) $$@
ZIPS += $(OUT)/$(1)-latest.zip $(OUT)/$(1)-$(call date,$(1)).zip
endef

$(foreach p,$(ZIP_BINS),$(eval $(call ZIP_template,$(p))))

zips: $(ZIPS)

incoming:
	./import-incoming.sh

-include local.mk

.SECONDARY: $(addprefix $(OUT)/,$(addsuffix .json,$(PACKS)))
.DELETE_ON_ERROR:
