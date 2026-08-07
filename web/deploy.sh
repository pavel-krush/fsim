#!/bin/sh
# Builds the browser version and publishes it to the gh-pages branch by hand.
#
# This is the fallback, not the usual route: .github/workflows/pages.yml is what
# publishes on a push to master. It exists because the Pages deployment queue does
# sometimes stall — one afternoon it sat in `deployment_queued` for ten minutes and
# timed out, on GitHub's own bot as well as on ours — and because a deploy you can
# run yourself is worth having when someone else's queue is the problem.
#
# It only serves once the source is pointed at the branch rather than at Actions:
#
#   gh api -X PUT repos/OWNER/REPO/pages -f build_type=legacy \
#     -f 'source[branch]=gh-pages' -f 'source[path]=/'
#
# and back again with build_type=workflow.
#
# The branch is replaced rather than added to: the commit has no parent and the push
# is a force, so the history is always exactly one commit and last week's wasm is not
# still in the repository. Nothing is checked out and the working tree is untouched —
# the tree is built with plumbing against a temporary index.
#
set -e
cd "$(dirname "$0")/.."

./web/build.sh

files="index.html wasm_exec.js fsim.wasm .nojekyll"
# A path that does not exist yet: git wants to create the index itself, and an
# empty file is not a valid one — "index file smaller than expected".
work=$(mktemp -d)
index="$work/index"
trap 'rm -rf "$work"' EXIT

# -f because the two built artefacts are gitignored, which is the point of them.
GIT_INDEX_FILE="$index" git --work-tree=web add -f -- $files
tree=$(GIT_INDEX_FILE="$index" git write-tree)
from=$(git rev-parse --short HEAD)
commit=$(git commit-tree "$tree" -m "fsim in a browser, built from $from")

git push --force origin "$commit:refs/heads/gh-pages"
echo "published $from as $commit -> gh-pages"
