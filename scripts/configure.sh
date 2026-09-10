#!/bin/sh
set -eu

skill_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_file=$skill_root/config.json

printf 'Feishu App ID: '
IFS= read -r app_id
case "$app_id" in
  cli_[A-Za-z0-9_-]*) ;;
  *) printf '%s\n' 'App ID should start with cli_' >&2; exit 2 ;;
esac

printf 'Feishu App Secret (input hidden): '
stty -echo
IFS= read -r app_secret
stty echo
printf '\n'
case "$app_secret" in
  *[!A-Za-z0-9_-]*|'') printf '%s\n' 'App Secret format is invalid' >&2; exit 2 ;;
esac

umask 077
printf '{\n  "app_id": "%s",\n  "app_secret": "%s",\n  "oauth_redirect_uri": "http://127.0.0.1:8080/callback",\n  "oauth_scope": "space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access",\n  "api_base": "https://open.feishu.cn",\n  "http_timeout_seconds": 30\n}\n' "$app_id" "$app_secret" > "$config_file"
chmod 600 "$config_file"
printf 'Configured: %s\n' "$config_file"
printf '%s\n' 'The config is saved in the Skill root. Next: run scripts/feishu-meetings.sh oauth-login --full and sign in with the current user account.'
unset app_secret
