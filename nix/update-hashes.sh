#!/usr/bin/env bash
# Recompute nix/navidrome.nix's Go vendorHash and npm hash so the fork builds
# after upstream dependency changes. Needs Nix with flakes. Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")/.."

nav=nix/navidrome.nix
fake="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
nixf=(--extra-experimental-features nix-command --extra-experimental-features flakes)

npm_hash=$(nix "${nixf[@]}" run nixpkgs#prefetch-npm-deps -- ui/package-lock.json)

# Force a mismatch so the builder prints the correct vendor hash.
sed -i -E "s#(vendorHash = \")[^\"]*(\")#\1${fake}\2#" "$nav"
vendor_hash=$(nix "${nixf[@]}" build .#navidrome-dl.goModules --no-link 2>&1 | awk '/got:/ {print $2}')

if [ -z "${vendor_hash:-}" ]; then
  echo "could not determine vendorHash" >&2
  exit 1
fi

sed -i -E "s#(vendorHash = \")[^\"]*(\")#\1${vendor_hash}\2#" "$nav"
sed -i -E "s#(^ *hash = \")[^\"]*(\")#\1${npm_hash}\2#" "$nav"

echo "vendorHash = $vendor_hash"
echo "npm hash   = $npm_hash"
