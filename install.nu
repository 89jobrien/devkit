#!/usr/bin/env nu

let devkit_version = ($env | get DEVKIT_VERSION? | default "v0.0.1")

# devkit installer

if (which go | is-empty) {
    print -e "error: go is not installed. Please install Go and try again."
    exit 1
}

gum style --bold --border double --padding "1 2" "devkit installer"

# Collect project info
let default_name = (pwd | path basename)
let project_name = (gum input --placeholder $default_name --prompt "Project name: " --value $default_name)
let project_description = (gum input --placeholder "A short description of your project" --prompt "Project description: ")

# CI platform selection
print "Select CI platform(s):"
let ci_platforms = (gum choose --no-limit github gitea none | lines)

# Component selection
print "Select components to enable:"
let components = (gum choose --no-limit --selected "council,review,meta,ci_agent,diagnose" council review meta ci_agent diagnose | lines)

# Review focus
let review_focus = (gum input --placeholder "security, performance, correctness" --prompt "Review focus: ")

# Derive booleans from selections
let has_council  = ($components | any { |it| $it == "council" })
let has_review   = ($components | any { |it| $it == "review" })
let has_meta     = ($components | any { |it| $it == "meta" })
let has_ci_agent = ($components | any { |it| $it == "ci_agent" })
let has_diagnose = ($components | any { |it| $it == "diagnose" })

# Install binaries
gum spin --spinner dot --title "Installing devkit..." -- go install $"github.com/89jobrien/devkit/cmd/devkit@($devkit_version)"

if $has_ci_agent {
    gum spin --spinner dot --title "Installing ci-agent..." -- go install $"github.com/89jobrien/devkit/cmd/ci-agent@($devkit_version)"
}

if $has_meta {
    gum spin --spinner dot --title "Installing meta..." -- go install $"github.com/89jobrien/devkit/cmd/meta@($devkit_version)"
}

# Build ci_platforms TOML array
let ci_filtered = ($ci_platforms | where { |it| $it != "" and $it != "none" })
let ci_toml_array = if ($ci_filtered | is-empty) {
    "[]"
} else {
    let quoted = ($ci_filtered | each { |it| $"\"($it)\"" } | str join ", ")
    $"[($quoted)]"
}

let install_date = (date now | format date "%Y-%m-%d")

# Write .devkit.toml
let toml_content = $"[project]
name        = \"($project_name)\"
description = \"($project_description)\"
install_date = \"($install_date)\"
ci_platforms = ($ci_toml_array)

[components]
council  = ($has_council)
review   = ($has_review)
meta     = ($has_meta)
ci_agent = ($has_ci_agent)
diagnose = ($has_diagnose)

[review]
focus = \"($review_focus)\"

[council]
mode = \"core\"

[diagnose]
# log_cmd = \"journalctl -n 200 --no-pager\"   # uncomment and customize if needed
# service = \"\"                                 # focus on a specific service

[providers]
# primary            = \"anthropic\"   # anthropic | openai | gemini
# fast_model         = \"\"            # override per-tier model
# balanced_model     = \"\"
# large_context_model = \"\"
# coding_model       = \"\"
"

$toml_content | save -f .devkit.toml
print "Wrote .devkit.toml"

# Copy or download CI yml files
def copy_ci_file [platform: string] {
    if $platform == "none" { return }
    let dest_dir = if $platform == "gitea" { ".gitea/workflows" } else { ".github/workflows" }
    mkdir $dest_dir
    let local_src = $"ci/($platform).yml"
    if ($local_src | path exists) {
        cp $local_src $"($dest_dir)/ci.yml"
        print $"Copied ($local_src) -> ($dest_dir)/ci.yml"
    } else {
        let url = $"https://raw.githubusercontent.com/89jobrien/devkit/($devkit_version)/ci/($platform).yml"
        if (which curl | is-not-empty) {
            run-external "curl" "-fsSL" $url "-o" $"($dest_dir)/ci.yml"
        } else {
            run-external "wget" "-qO" $"($dest_dir)/ci.yml" $url
        }
        print $"Downloaded ($platform) CI template -> ($dest_dir)/ci.yml"
    }
}

for p in $ci_filtered {
    copy_ci_file $p
}

# Git hooks
if (".git" | path exists) {
    if $has_review {
        "#!/bin/sh
if [ \"${DEVKIT_SKIP_HOOKS:-0}\" = \"1\" ]; then
  exit 0
fi
devkit review --base HEAD
" | save -f .git/hooks/pre-commit
        run-external "chmod" "+x" ".git/hooks/pre-commit"
        print "Installed git pre-commit hook (devkit review)"
    }

    if $has_council {
        "#!/bin/sh
if [ \"${DEVKIT_SKIP_HOOKS:-0}\" = \"1\" ]; then
  exit 0
fi
if [ -z \"${ANTHROPIC_API_KEY:-}\" ] && [ -z \"${OPENAI_API_KEY:-}\" ]; then
  echo \"devkit: no API key set, skipping council\"
  exit 0
fi
BASE=$(git merge-base HEAD origin/main 2>/dev/null || echo \"HEAD~10\")
if [ \"$BASE\" = \"$(git rev-parse HEAD 2>/dev/null)\" ]; then
  BASE=\"HEAD~10\"
fi
devkit council --base \"$BASE\" || echo \"devkit: council failed (non-blocking)\"
" | save -f .git/hooks/pre-push
        run-external "chmod" "+x" ".git/hooks/pre-push"
        print "Installed git pre-push hook (devkit council)"
    }
}

# Claude Code hooks
if $has_review {
    let claude_settings = ".claude/settings.json"
    mkdir .claude
    let hook_json = '{
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
    if ($claude_settings | path exists) {
        if (which python3 | is-not-empty) {
            let hooks_tmp = (mktemp)
            let py_tmp = (mktemp --suffix .py)
            $hook_json | save -f $hooks_tmp
"import sys, json
settings_path = sys.argv[1]
hooks_path = sys.argv[2]
with open(settings_path) as f:
    existing = json.load(f)
with open(hooks_path) as f:
    new_hooks = json.load(f)
existing.setdefault('hooks', {})
existing['hooks'].update(new_hooks['hooks'])
with open(settings_path, 'w') as f:
    json.dump(existing, f, indent=2)
    f.write('\n')
" | save -f $py_tmp
            run-external "python3" $py_tmp $claude_settings $hooks_tmp
            rm $hooks_tmp $py_tmp
            print $"Merged Claude Code hooks into ($claude_settings)"
        } else {
            $hook_json | save -f $claude_settings
            print $"Wrote Claude Code hooks to ($claude_settings)"
        }
    } else {
        $hook_json | save -f $claude_settings
        print $"Wrote Claude Code hooks to ($claude_settings)"
    }
}

# Summary
print ""
gum style --bold "Installation complete."
print $"  Project:    ($project_name)"
print $"  Components: ($components | str join ', ')"
print $"  CI:         ($ci_platforms | str join ', ')"
print $"  Config:     .devkit.toml"
