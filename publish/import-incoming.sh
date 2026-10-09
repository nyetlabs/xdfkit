#!/bin/sh
# Import each KP or XDF in testdata/local/incoming into its pack's corpus
# JSON (make import), then move it to incoming/imported/ so a later run
# can't import it over newer work. See README.md.
#
# The pack is the longest images.tsv stem the file name starts with,
# followed by "." or "-" (8D0907551K.kp, 8D0907551K-0001-fixed.xdf). An XDF
# uses NAME.meta.json next to it, else build/publish/PACK.meta.json. A corpus
# JSON whose stamp is edited is skipped unless FORCE=1. A second file for
# one pack waits for the next run.
#
# INCOMING overrides the directory. CORPUS is passed on to make.
set -eu
cd "$(dirname "$0")"
root=$(cd .. && pwd)
in=${INCOMING:-$root/testdata/local/incoming}
tsv=$root/testdata/archive/ecuxplot/images.tsv
corpus=${CORPUS:-$root/corpus}
xdfkit=$root/build/xdfkit
make=${MAKE:-make}

test -d "$in" || { echo "$in: no such directory" >&2; exit 1; }
test -x "$xdfkit" || $make -C "$root" build
stems=$(awk -F'\t' 'NR > 1 { print length($1) "\t" $1 }' "$tsv" | sort -rn | cut -f2)

status=0
found=0
seen=
for f in "$in"/*; do
	test -f "$f" || continue
	name=$(basename "$f")
	case $name in
	*.meta.json) continue ;;
	*.kp | *.KP | *.xdf | *.XDF) ;;
	*)
		echo "$name: skipped, not a KP or XDF" >&2
		continue
		;;
	esac
	found=1
	pack=
	for s in $stems; do
		case $name in
		"$s".* | "$s"-*)
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
	case " $seen " in
	*" $pack "*)
		echo "$name: skipped, another file for $pack was imported in this run" >&2
		status=1
		continue
		;;
	esac
	seen="$seen $pack"
	image=$(awk -F'\t' -v p="$pack" '$1 == p { print $2 }' "$tsv")
	def=$corpus/defs/$image.json
	if test -f "$def" && test "${FORCE:-}" != 1 && "$xdfkit" verify "$def" | grep -Eq ': (edited|mixed)'; then
		echo "$name: skipped, $def is hand edited (FORCE=1 imports anyway)" >&2
		status=1
		continue
	fi
	meta=
	case $name in
	*.xdf | *.XDF)
		if test -f "${f%.*}.meta.json"; then
			meta=${f%.*}.meta.json
		elif test -f "$root/build/publish/$pack.meta.json"; then
			meta=$root/build/publish/$pack.meta.json
			echo "$name: using build/publish/$pack.meta.json" >&2
		else
			echo "$name: no metadata file; what XDF can't hold is lost" >&2
		fi
		;;
	esac
	echo "$name -> $pack ($image)"
	if ! $make --no-print-directory import PACK="$pack" SRC="$f" META="$meta"; then
		status=1
		continue
	fi
	mkdir -p "$in/imported"
	mv "$f" "$in/imported/"
	case $meta in
	"$in"/*) mv "$meta" "$in/imported/" ;;
	esac
done
test "$found" = 1 || echo "nothing to import in $in"
exit $status
