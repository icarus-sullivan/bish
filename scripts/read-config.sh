#!/bin/sh
# Reads a scalar or one-level-nested value ("version" or "app.name") out of
# release.config.yaml without pulling in a YAML parser dependency.
set -e
key="$1"
file="$(dirname "$0")/../release.config.yaml"

case "$key" in
  *.*)
    sec="${key%%.*}:"
    field="${key#*.}:"
    awk -v sec="$sec" -v field="$field" '
      $0 == sec { insec=1; next }
      /^[a-zA-Z_]+:$/ { insec=0 }
      insec && $1 == field { sub(/^[^:]+:[ \t]*/, ""); print; exit }
    ' "$file"
    ;;
  *)
    field="$key:"
    awk -v field="$field" '$1==field{sub(/^[^:]+:[ \t]*/,"");print;exit}' "$file"
    ;;
esac
