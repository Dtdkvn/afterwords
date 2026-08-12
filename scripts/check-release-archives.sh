#!/usr/bin/env bash
set -euo pipefail

dist_dir="${1:-dist}"
shopt -s nullglob
archives=("${dist_dir}"/*.tar.gz)
if (( ${#archives[@]} == 0 )); then
  echo "No release archives found in ${dist_dir}." >&2
  exit 1
fi

required=(
  LICENSE
  README.md
  SECURITY.md
  CONTRIBUTING.md
  docs/LAUNCH.md
  docs/DECISIONS.md
  .env.example
  scripts/record-demo.md
)

for archive in "${archives[@]}"; do
  listing="$(tar -tzf "${archive}")"
  for target in "${required[@]}"; do
    escaped="${target//./\.}"
    if ! grep -Eq "(^|/)${escaped}$" <<<"${listing}"; then
      echo "${archive} is missing README/release target: ${target}" >&2
      exit 1
    fi
  done
done

echo "Validated ${#archives[@]} release archive(s)."
