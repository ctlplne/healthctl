#!/bin/sh
# Collect upstream license files for the Go code actually linked into the executable.
set -eu
dest=$1
binary=$2
mkdir -p "$dest"
goroot=$(go env GOROOT)
license="$goroot/LICENSE"
# Homebrew keeps the upstream license one directory above libexec.
if [ ! -f "$license" ] && [ "$(basename "$goroot")" = libexec ]; then
    license="$goroot/../LICENSE"
fi
cp "$license" "$dest/go-LICENSE"
go version -m "$binary" > "$dest/build-info.tmp"
awk -v name="$(basename "$binary")" 'NR == 1 {print name ": " $NF; next} {print}' \
    "$dest/build-info.tmp" > "$dest/build-info.txt"
rm "$dest/build-info.tmp"
awk '$1 == "dep" { print $2, $3 }' "$dest/build-info.txt" > "$dest/modules.txt"
while read -r module version; do
    dir=$(go list -m -f '{{.Dir}}' "$module")
    target="$dest/$module@$version"
    mkdir -p "$target"
    find "$dir" -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice' -o -iname 'patents' \) \
        -exec sh -c 'base=$1; target=$2; shift 2; for path do
            relative=${path#"$base"/}; mkdir -p "$target/$(dirname "$relative")"; cp "$path" "$target/$relative"
        done' sh "$dir" "$target" {} +
    test -n "$(find "$target" -type f -print -quit)"
done < "$dest/modules.txt"
