#!/usr/bin/env bash
# Scaffold a client dashboard config directory.
#
#   make dashboard SLUG=acme
#   bash .agents/skills/sk-clients/scripts/new-dashboard.sh acme [config dir]
#
# Copies the skill's template into config/dashboards/<slug>/, substituting the
# slug everywhere the templates reference it, and refuses to touch a directory
# that already exists. It writes CONFIG ONLY: running the instance and routing
# the prefix is deployment work — see ../guides/deploy.md.
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

# Same charset the server enforces for HOMEPAGE_BASE_PATH: anything else makes
# the instance refuse to start, so it is better refused here.
if [[ ! "$slug" =~ ^[A-Za-z0-9._~-]+$ ]]; then
  echo "invalid slug '$slug': allowed characters are A-Z a-z 0-9 . _ ~ -" >&2
  echo "(nested prefixes like /clients/acme work, but create them by hand)" >&2
  exit 1
fi

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
echo "next:"
echo "  1. edit settings.yaml / services.yaml / widgets.yaml"
echo "  2. to require login: fill auth.yaml.example, export its variables, then"
echo "     rename it to auth.yaml. Left as .example, the dashboard is public."
echo "     - session.cookieName must be unique on this hostname"
echo "     - google.redirectURL must carry /$slug AND be registered in the Google console"
echo "  3. run an instance:  HOMEPAGE_CONFIG_DIR=$target HOMEPAGE_BASE_PATH=/$slug"
echo "  4. route /$slug to it WITHOUT stripping the prefix, and point its"
echo "     healthcheck at /$slug/api/healthcheck"
echo ""
echo "details: $SKILL_DIR/guides/deploy.md"
