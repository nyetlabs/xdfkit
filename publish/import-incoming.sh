#!/bin/sh
# Import KP and XDF files into their packs' corpus JSON: the files given, else
# every one in testdata/local/incoming. See README.md.
#
# The pack is the longest images.tsv stem the file name starts with, followed
# by "." or "-" (8D0907551K.kp, 8D0907551K-0001-fixed.xdf). An XDF uses
# NAME.meta.json next to it, else build/publish/PACK.meta.json. The corpus JSON
# is rewritten only when the definitions or their origin changed; files from
# incoming then move to incoming/imported/, so a later run can't import them
# over newer work. A second file for one pack waits for the next run. A subset
# (provenance.subset, as in PACK-tuner.xdf, even renamed) is refused, and so
# is origin located (maps found by a program such as me7info).
#
# Environment: INCOMING (the directory), CORPUS (an ecu-corpus checkout;
# default ../ecu-corpus beside this repo, else the submodule),
# ORIGIN (damos, a2l or hand; default the corpus JSON's, and hand becomes damos
# from DAMOS_MAPS=3000 maps, docs/corpus.md), FORCE=1 (import over a corpus
# JSON whose stamp is edited).
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
in=${INCOMING:-$root/testdata/local/incoming}
tsv=$root/testdata/archive/ecuxplot/images.tsv
corpus=${CORPUS:-$(cd "$root/../ecu-corpus" 2>/dev/null && pwd || echo "$root/corpus")}
xdfkit=$root/build/xdfkit
test -x "$xdfkit" || make -C "$root" build
stems=$(awk -F'\t' 'NR > 1 { print length($1) "\t" $1 }' "$tsv" | sort -rn | cut -f2)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The definitions and origin, without the stamp and the rest of the provenance.
model() { jq -S 'del(.stamp, .provenance.format, .provenance.file, .provenance.sha256)' "$1"; }
convert() { "$xdfkit" --force ${meta:+--meta "$meta"} --origin "$1" "$f" "$tmp/new.json"; }
converted() { jq -r .provenance.origin "$tmp/new.json"; }

test $# -gt 0 || set -- "$in"/*.kp "$in"/*.KP "$in"/*.xdf "$in"/*.XDF
status=0
seen=
for f; do
	test -f "$f" || continue
	name=$(basename "$f")
	pack=
	for s in $stems; do
		case $name in "$s".* | "$s"-*)
			pack=$s
			break
			;;
		esac
	done
	if test -z "$pack"; then
		echo "$name: no pack in images.tsv matches the name" >&2
		status=1
		continue
	fi
	case " $seen " in *" $pack "*)
		echo "$name: skipped, another file for $pack was imported in this run" >&2
		status=1
		continue
		;;
	esac
	seen="$seen $pack"
	def=$corpus/defs/$(awk -F'\t' -v p="$pack" '$1 == p { print $2 }' "$tsv").json
	if test -f "$def" && test "${FORCE:-}" != 1 && "$xdfkit" verify "$def" | grep -Eq ': (edited|mixed)'; then
		echo "$name: skipped, $def is hand edited (FORCE=1 imports anyway)" >&2
		status=1
		continue
	fi
	meta=
	case $name in *.xdf | *.XDF)
		for meta in "${f%.*}.meta.json" "$root/build/publish/$pack.meta.json" ""; do
			test -z "$meta" || test -f "$meta" && break
		done
		echo "$name: metadata ${meta:-missing, so what XDF cannot hold is lost}" >&2
		;;
	esac
	origin=${ORIGIN:-}
	test -n "$origin" || ! test -f "$def" || origin=$(jq -r '.provenance.origin // empty' "$def")
	convert "$origin" || { status=1; continue; }
	if jq -e '.provenance.subset' "$tmp/new.json" >/dev/null; then
		echo "$name: skipped, a $(jq -r .provenance.subset.kind "$tmp/new.json") subset (PACK-tuner.xdf) can't replace the full definition" >&2
		status=1
		continue
	fi
	if test "$(jq -r .provenance.origin "$tmp/new.json")" = located; then
		echo "$name: skipped, maps located by a program (origin located) aren't a corpus source" >&2
		status=1
		continue
	fi
	n=$(jq '.objects | length' "$tmp/new.json")
	if test -z "${ORIGIN:-}" && test "$(jq -r .provenance.origin "$tmp/new.json")" = hand && test "$n" -ge "${DAMOS_MAPS:-3000}"; then
		echo "$name: $n maps, origin damos"
		convert damos || { status=1; continue; }
	fi
	if test "$(jq -r .provenance.origin "$tmp/new.json")" = damos; then
		recip='[.objects[] | select(.value.conversion.reciprocal) | .key]'
		if test -f "$def"; then
			lost=$(jq -rn --argjson old "$(jq -c "$recip" "$def")" --argjson new "$(jq -c "$recip" "$tmp/new.json")" '$old - $new | "\(length) \(.[:5] | join(" "))"')
			test "${lost%% *}" = 0 || echo "$name: WARNING: ${lost%% *} maps lose the reciprocal conversion $def has (${lost#* } ...); WinOLS's DAMOS import drops them (docs/corpus.md)" >&2
		fi
		test "$(jq "$recip | length" "$tmp/new.json")" != 0 ||
			echo "$name: WARNING: no reciprocal conversions; WinOLS's DAMOS import sets them to factor 1, so time constants (ZK*) read raw (docs/corpus.md)" >&2
	fi
	model "$tmp/new.json" >"$tmp/new.model"
	if test -f "$def" && model "$def" | cmp -s - "$tmp/new.model"; then
		echo "$name: $pack unchanged"
	else
		cp "$tmp/new.json" "$def"
		echo "$name: updated $def; commit it in the corpus"
	fi
	case $f in "$in"/*)
		mkdir -p "$in/imported"
		mv "$f" "$in/imported/"
		case $meta in "$in"/*) mv "$meta" "$in/imported/" ;; esac
		;;
	esac
done
exit $status
