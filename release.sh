#!/bin/sh
# Cut a release: build every target with the version and amarit.json stamped
# in, write SHA256SUMS and releases.json, and publish them as a GitHub
# release. The manifest carries earlier releases over from the last one.
#
#   ./release.sh v0.1.0
set -eu
VERSION=${1:?usage: release.sh vX.Y.Z}
DIST=dist
NOTES=$(mktemp)

go vet ./... && go test ./...
rm -rf "$DIST" && mkdir -p "$DIST"
STAMP=$(amarit stamp -version "$VERSION")
for target in $(sed -n 's/.*"targets": \[\(.*\)\].*/\1/p' amarit.json | tr -d '",'); do
	GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 go build -trimpath \
		-ldflags "$STAMP" -o "$DIST/keykosh-${target%/*}-${target#*/}" .
done
(cd "$DIST" && shasum -a 256 keykosh-* > SHA256SUMS)
gh release download --pattern releases.json -O "$DIST/releases.json" 2>/dev/null || true
amarit release -version "$VERSION" -dist "$DIST" -out "$DIST/releases.json"

awk -v v="## $VERSION" '$0 == v {on = 1; next} /^## / {on = 0} on' CHANGELOG.md > "$NOTES"
gh release create "$VERSION" "$DIST"/* --title "$VERSION" --notes-file "$NOTES"
rm -f "$NOTES"
