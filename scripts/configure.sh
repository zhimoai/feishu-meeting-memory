#!/bin/sh
set -eu

if [ -n "${XDG_CONFIG_HOME:-}" ]; then
  config_root=$XDG_CONFIG_HOME
else
  config_root=$HOME/.config
fi
config_dir=$config_root/feishu-meeting-memory
config_file=$config_dir/config.json

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
mkdir -p "$config_dir"
printf '{\n  "app_id": "%s",\n  "app_secret": "%s",\n  "oauth_redirect_uri": "http://127.0.0.1:8080/callback",\n  "oauth_scope": "space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access",\n  "api_base": "https://open.feishu.cn",\n  "http_timeout_seconds": 30\n}\n' "$app_id" "$app_secret" > "$config_file"
chmod 600 "$config_file"
printf 'Configured: %s\n' "$config_file"
printf '%s\n' 'Next: run scripts/feishu-meetings.sh oauth-login --full and sign in with the current user account.'
unset app_secret
