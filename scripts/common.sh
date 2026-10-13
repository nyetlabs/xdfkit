# Shared by the xdfkit helper scripts. The Makefile is the interface.
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)

ldflags() {
	printf -- '-X go.nyet.org/xdfkit/api.Version=%s' "$1"
}
