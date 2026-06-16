#!/bin/sh
# Build the pinned zooid (docs/operations/updating.md). Cached on the commit
# hash from ZOOID_VERSION; invalidated by bumping the pin.
set -eu
cd "$(dirname "$0")/.."

. ./ZOOID_VERSION 2>/dev/null || {
  ZOOID_REPO=$(grep '^ZOOID_REPO=' ZOOID_VERSION | cut -d= -f2)
  ZOOID_COMMIT=$(grep '^ZOOID_COMMIT=' ZOOID_VERSION | cut -d= -f2)
}

# Per-arch cache so cross-compiled artifacts don't collide.
arch="${GOARCH:-$(${GO:-go} env GOARCH)}"
cache=".cache/zooid-${ZOOID_COMMIT}-${arch}"
mkdir -p bin .cache

if [ ! -x "${cache}/zooid" ]; then
  echo "building zooid @ ${ZOOID_COMMIT} (${arch})"
  rm -rf "${cache}/src"
  git clone "${ZOOID_REPO}" "${cache}/src"
  git -C "${cache}/src" checkout "${ZOOID_COMMIT}"
  # GOARCH/CC are honoured from the environment for cross builds.
  (cd "${cache}/src" && CGO_ENABLED=1 ${GO:-go} build -o ../zooid ./cmd/relay)
  rm -rf "${cache}/src"
fi

ln -sf "../${cache}/zooid" bin/zooid
echo "bin/zooid -> ${cache}/zooid"
