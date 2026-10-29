#!/bin/sh
set -eu
version=$(GOTOOLCHAIN=local go env GOVERSION)
case "$version" in
  go1.*) ;;
  *) echo "ERROR: Go 1.27.3 or newer is required; found $version" >&2; exit 1 ;;
esac
number=${version#go1.}
minor=${number%%.*}
rest=${number#*.}
patch=${rest%%[!0-9]*}
if [ "$patch" != "$rest" ]; then
 echo "ERROR: use a released Go toolchain, not $version" >&2; exit 1
fi
case "$minor:$patch" in *[!0-9:]*|*:) echo "ERROR: use a released Go toolchain, not $version" >&2; exit 1;; esac
if [ "$minor" -lt 27 ] || { [ "$minor" -eq 27 ] && [ "$patch" -lt 3 ]; }; then
  echo "ERROR: MyGo v0.7.1 requires Go >=1.27.3; installed: $version" >&2
  exit 1
fi
printf 'Toolchain: %s\n' "$version"
