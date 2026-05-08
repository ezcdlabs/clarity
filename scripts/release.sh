#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# release.sh — interactive version picker and tag pusher for git-clarity
# Usage: ./scripts/release.sh
# ---------------------------------------------------------------------------

# Ensure we're on main and up to date.
current_branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$current_branch" != "main" ]; then
  echo "error: must be on main to release (currently on '$current_branch')" >&2
  exit 1
fi

echo "Fetching origin..."
git fetch origin main --tags --quiet

# Ensure local main is in sync with origin/main.
local_sha=$(git rev-parse HEAD)
remote_sha=$(git rev-parse origin/main)
if [ "$local_sha" != "$remote_sha" ]; then
  if git merge-base --is-ancestor "$remote_sha" "$local_sha"; then
    echo "error: local main is ahead of origin/main — push your commits first" >&2
  else
    echo "error: local main is behind origin/main — pull before releasing" >&2
  fi
  exit 1
fi

# Find latest semver tag.
latest=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)

if [ -z "$latest" ]; then
  echo "No existing tags found — this will be the first release."
  latest="v0.0.0"
fi

# Parse semver components.
version="${latest#v}"
IFS='.' read -r major minor patch <<< "$version"

next_patch="v${major}.${minor}.$((patch + 1))"
next_minor="v${major}.$((minor + 1)).0"
next_major="v$((major + 1)).0.0"

# Show context.
echo ""
echo "Current version : $latest"
echo ""
echo "Commits since $latest:"
if git log "${latest}..HEAD" --oneline 2>/dev/null | grep -q .; then
  git log "${latest}..HEAD" --oneline | head -20
else
  echo "  (none — you may be re-releasing the same commit)"
fi

echo ""
echo "Pick next version:"
echo "  1) $next_patch  (patch — bug fixes)"
echo "  2) $next_minor  (minor — new features, backwards compatible)"
echo "  3) $next_major  (major — breaking changes)"
echo "  4) custom"
echo ""
read -rp "Choice [1]: " choice
choice="${choice:-1}"

case "$choice" in
  1) next="$next_patch" ;;
  2) next="$next_minor" ;;
  3) next="$next_major" ;;
  4)
    read -rp "Version (e.g. v1.2.3): " next
    ;;
  *)
    echo "error: invalid choice '$choice'" >&2
    exit 1
    ;;
esac

# Validate format.
if ! echo "$next" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "error: '$next' is not a valid semver tag (expected vX.Y.Z)" >&2
  exit 1
fi

# Check tag doesn't already exist.
if git rev-parse "$next" &>/dev/null; then
  echo "error: tag '$next' already exists" >&2
  exit 1
fi

echo ""
echo "Will create and push tag: $next"
read -rp "Confirm? [y/N]: " confirm
if [[ "${confirm,,}" != "y" ]]; then
  echo "Aborted."
  exit 0
fi

git tag "$next"
git push origin "$next"

echo ""
echo "Tagged $next and pushed. GitHub Actions will build and publish the release:"
echo "  https://github.com/ezcdlabs/clarity/actions"
