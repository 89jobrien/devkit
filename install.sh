#!/usr/bin/env bash
set -eu

DEVKIT_VERSION="${DEVKIT_VERSION:-v0.0.1}"

# devkit installer

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is not installed. Please install Go and try again." >&2
  exit 1
fi

gum style --bold --border double --padding "1 2" "devkit installer"

# Collect project info
default_name="$(basename "$(pwd)")"
project_name="$(gum input --placeholder "$default_name" --prompt "Project name: " --value "$default_name")"
project_description="$(gum input --placeholder "A short description of your project" --prompt "Project description: ")"

# CI platform selection
echo "Select CI platform(s):"
ci_platforms="$(gum choose --no-limit github gitea none)"

# Component selection
echo "Select components to enable:"
components="$(gum choose --no-limit --selected council,review,meta,ci_agent,diagnose council review meta ci_agent diagnose)"

# Review focus
review_focus="$(gum input --placeholder "security, performance, correctness" --prompt "Review focus: ")"

# Derive booleans from selections
has_council=false
has_review=false
has_meta=false
has_ci_agent=false
has_diagnose=false
if echo "$components" | grep -q "council";  then has_council=true;  fi
if echo "$components" | grep -q "review";   then has_review=true;   fi
if echo "$components" | grep -q "meta";     then has_meta=true;     fi
if echo "$components" | grep -q "ci_agent"; then has_ci_agent=true; fi
if echo "$components" | grep -q "diagnose"; then has_diagnose=true; fi

# Install binaries
gum spin --spinner dot --title "Installing devkit..." -- \
  go install github.com/89jobrien/devkit/cmd/devkit@${DEVKIT_VERSION}

if [ "$has_ci_agent" = true ]; then
  gum spin --spinner dot --title "Installing ci-agent..." -- \
    go install github.com/89jobrien/devkit/cmd/ci-agent@${DEVKIT_VERSION}
fi

# Build ci_platforms TOML array
ci_toml_array="["
first=true
while IFS= read -r p; do
  [ -z "$p" ] && continue
  if [ "$p" = "none" ]; then continue; fi
  if [ "$first" = true ]; then
    ci_toml_array="${ci_toml_array}\"$p\""
    first=false
  else
    ci_toml_array="${ci_toml_array}, \"$p\""
  fi
done <<EOF
$(echo "$ci_platforms" | tr ' ' '\n')
EOF
ci_toml_array="${ci_toml_array}]"

install_date="$(date +%Y-%m-%d)"

# Write .devkit.toml
cat > .devkit.toml <<TOML
[project]
name        = "$project_name"
description = "$project_description"
install_date = "$install_date"
ci_platforms = $ci_toml_array

[components]
council  = $has_council
review   = $has_review
meta     = $has_meta
ci_agent = $has_ci_agent
diagnose = $has_diagnose

[review]
focus = "$review_focus"

[council]
mode = "core"

[diagnose]
# log_cmd = "journalctl -n 200 --no-pager"   # uncomment and customize if needed
# service = ""                                 # focus on a specific service
TOML

echo "Wrote .devkit.toml"

# Copy or download CI yml files
copy_ci_file() {
  platform="$1"
  [ "$platform" = "none" ] && return
  dest_dir=".github/workflows"
  if [ "$platform" = "gitea" ]; then
    dest_dir=".gitea/workflows"
  fi
  mkdir -p "$dest_dir"
  if [ -f "ci/${platform}.yml" ]; then
    cp "ci/${platform}.yml" "$dest_dir/ci.yml"
    echo "Copied ci/${platform}.yml -> $dest_dir/ci.yml"
  else
    url="https://raw.githubusercontent.com/89jobrien/devkit/${DEVKIT_VERSION}/ci/${platform}.yml"
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "$url" -o "$dest_dir/ci.yml"
    else
      wget -qO "$dest_dir/ci.yml" "$url"
    fi
    echo "Downloaded $platform CI template -> $dest_dir/ci.yml"
  fi
}

while IFS= read -r p; do
  [ -z "$p" ] && continue
  copy_ci_file "$p"
done <<EOF
$(echo "$ci_platforms" | tr ' ' '\n')
EOF

# Git hooks
if [ -d ".git" ]; then
  if [ "$has_review" = true ]; then
    cat > .git/hooks/pre-commit <<'HOOK'
#!/bin/sh
if [ "${DEVKIT_SKIP_HOOKS:-0}" = "1" ]; then
  exit 0
fi
devkit review --base HEAD
HOOK
    chmod +x .git/hooks/pre-commit
    echo "Installed git pre-commit hook (devkit review)"
  fi

  if [ "$has_council" = true ]; then
    cat > .git/hooks/pre-push <<'HOOK'
#!/bin/sh
if [ "${DEVKIT_SKIP_HOOKS:-0}" = "1" ]; then
  exit 0
fi
if [ -z "${ANTHROPIC_API_KEY:-}" ] && [ -z "${OPENAI_API_KEY:-}" ]; then
  echo "devkit: no API key set, skipping council"
  exit 0
fi
BASE=$(git merge-base HEAD origin/main 2>/dev/null || echo "HEAD~10")
if [ "$BASE" = "$(git rev-parse HEAD 2>/dev/null)" ]; then
  BASE="HEAD~10"
fi
devkit council --base "$BASE" || echo "devkit: council failed (non-blocking)"
HOOK
    chmod +x .git/hooks/pre-push
    echo "Installed git pre-push hook (devkit council)"
  fi
fi

# Claude Code hooks
if [ "$has_review" = true ]; then
  claude_settings=".claude/settings.json"
  mkdir -p .claude
  hook_json='{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "devkit review --base HEAD~1 2>/dev/null || true"
          }
        ]
      }
    ]
  }
}'
  if [ -f "$claude_settings" ]; then
    # Merge hooks key using python if available, else overwrite
    if command -v python3 >/dev/null 2>&1; then
      python3 - "$claude_settings" "$hook_json" <<'PY'
import sys, json
settings_path = sys.argv[1]
new_hooks = json.loads(sys.argv[2])
with open(settings_path) as f:
    existing = json.load(f)
existing.setdefault("hooks", {})
existing["hooks"].update(new_hooks["hooks"])
with open(settings_path, "w") as f:
    json.dump(existing, f, indent=2)
    f.write("\n")
PY
      echo "Merged Claude Code hooks into $claude_settings"
    else
      echo "$hook_json" > "$claude_settings"
      echo "Wrote Claude Code hooks to $claude_settings"
    fi
  else
    echo "$hook_json" > "$claude_settings"
    echo "Wrote Claude Code hooks to $claude_settings"
  fi
fi

# Summary
echo ""
gum style --bold "Installation complete."
echo "  Project:    $project_name"
echo "  Components: $components"
echo "  CI:         $ci_platforms"
echo "  Config:     .devkit.toml"
