#!/usr/bin/env bash
# Scaffold a client dashboard config directory.
#
#   make dashboard SLUG=acme
#   bash .agents/skills/sk-clients/scripts/new-dashboard.sh acme [config dir]
#
# Copies the skill's template into config/dashboards/<slug>/, substituting the
# slug everywhere the templates reference it, and refuses to touch a directory
# that already exists.
#
# Creating the directory IS creating the dashboard: one process serves them all,
# and a running one starts serving /<slug> without a restart. There is nothing
# to deploy afterwards — see ../guides/deploy.md.
set -euo pipefail

SKILL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(cd "$SKILL_DIR/../../.." && pwd)"

slug="${1:-}"
config_root="${2:-${HOMEPAGE_CONFIG_DIR:-$REPO_DIR/config}}"

if [[ -z "$slug" ]]; then
  echo "usage: $(basename "$0") <slug> [config dir]" >&2
  echo "       the slug is the URL prefix: acme -> /acme" >&2
  exit 1
fi

# Same charset the server enforces for a slug: a directory it cannot validate is
# skipped with a log line rather than served, so it is better refused here.
if [[ ! "$slug" =~ ^[A-Za-z0-9._~-]+$ ]]; then
  echo "invalid slug '$slug': allowed characters are A-Z a-z 0-9 . _ ~ -" >&2
  exit 1
fi

# These would shadow the root dashboard's own routes.
case "$slug" in
  api|auth|static)
    echo "'$slug' is reserved: a dashboard with that name would shadow /$slug" >&2
    exit 1
    ;;
esac

target="$config_root/dashboards/$slug"
if [[ -e "$target" ]]; then
  echo "$target already exists — edit it instead of scaffolding over it" >&2
  exit 1
fi

mkdir -p "$(dirname "$target")"
cp -R "$SKILL_DIR/templates/dashboard" "$target"

# The templates are written for a dashboard called "acme"; make them this one's.
title="$(printf '%s' "${slug:0:1}" | tr '[:lower:]' '[:upper:]')${slug:1}"
while IFS= read -r -d '' f; do
  perl -pi -e "s/\\bacme\\b/$slug/g; s/\\bAcme\\b/$title/g" "$f"
done < <(find "$target" -type f -print0)

echo "created $target"
echo ""
echo "it is already being served at /$slug — a running process picks up the new"
echo "directory without a restart. No container, no proxy rule, no redirect URI."
echo ""
echo "next:"
echo "  1. edit settings.yaml / services.yaml / widgets.yaml"
echo "  2. to require login: fill auth.yaml.example, export its variables, then"
echo "     rename it to auth.yaml. Left as .example, the dashboard is PUBLIC."
echo "     google.redirectURL names the ROOT dashboard's callback, not /$slug's."
echo "  3. check it:  curl -s localhost:3000/$slug/api/services"
echo ""
echo "details: $SKILL_DIR/guides/auth.md and $SKILL_DIR/guides/deploy.md"
