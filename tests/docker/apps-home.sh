#!/bin/sh
# Neutral content for the apps to show: a small git repository (tig,
# lazygit) and a directory tree (ncdu, file managers, less).
set -eu
cd "$HOME"
git config --global user.name demo
git config --global user.email demo@example.invalid
git config --global init.defaultBranch main
mkdir -p repo tree/docs tree/src tree/data
cd repo
git init -q
for i in 1 2 3 4 5 6; do
	printf 'line %s\n' $(seq 1 "$i") >"file$i.txt"
	git add . && git commit -qm "Add file $i"
done
echo change >>file1.txt
cd "$HOME/tree"
for d in docs src data; do
	for i in 1 2 3 4; do
		seq 1 $((i * 200)) >"$d/$d-$i.txt"
	done
done
seq 1 2000 | sed 's/^/Sample text line /' >"$HOME/sample.txt"
