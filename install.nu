#!/usr/bin/env nu

let devkit_version = ($env | get DEVKIT_VERSION? | default "v0.0.1")

# devkit installer

# --- dependency preflight ---

def require [cmd: string, hint: string] {
    if (which $cmd | is-empty) {
        print -e $"error: ($cmd) is not installed. ($hint)"
        exit 1
    }
}

def warn_missing [cmd: string, feature: string] {
    if (which $cmd | is-empty) {
        print $"warning: ($cmd) not found — ($feature) will be skipped"
        false
    } else {
        true
    }
}

require "go"   "Install Go from https://go.dev/dl/ and try again."
require "gum"  "Install gum from https://github.com/charmbracelet/gum and try again."

let have_curl = (which curl | is-not-empty)
let have_wget = (which wget | is-not-empty)

# --- UI ---

gum style --bold --border double --padding "1 2" "devkit installer"

# Collect project info
let default_name = (pwd | path basename)
let project_name = (gum input --placeholder $default_name --prompt "Project name: " --value $default_name)
let project_description = (gum input --placeholder "A short description of your project" --prompt "Project description: ")

# CI platform selection
print "Select CI platform(s):"
let ci_platforms = (gum choose --no-limit github gitea none | lines | where { |it| $it != "" })

# Component selection
print "Select components to enable:"
let components = (gum choose --no-limit --selected "council,review,meta,ci_agent,diagnose" council review meta ci_agent diagnose | lines | where { |it| $it != "" })

# Review focus
let review_focus = (gum input --placeholder "security, performance, correctness" --prompt "Review focus: ")

# Derive booleans from selections
let has_council  = ($components | any { |it| $it == "council" })
let has_review   = ($components | any { |it| $it == "review" })
let has_meta     = ($components | any { |it| $it == "meta" })
let has_ci_agent = ($components | any { |it| $it == "ci_agent" })
let has_diagnose = ($components | any { |it| $it == "diagnose" })

# --- helpers ---

# Escape a string for use inside a TOML double-quoted value
def toml_escape [s: string] {
    $s | str replace --all '\\' '\\\\' | str replace --all '"' '\\"'
}

# Run a command and exit with a message on failure
def run_or_die [title: string, ...args: string] {
    let result = (do { run-external ...$args } | complete)
    if $result.exit_code != 0 {
        print -e $"error: ($title) failed \(exit ($result.exit_code)\)"
        print -e $result.stderr
        exit 1
    }
}

# --- install binaries ---

gum spin --spinner dot --title "Installing devkit..." -- go install $"github.com/89jobrien/devkit/cmd/devkit@($devkit_version)"
if $env.LAST_EXIT_CODE != 0 {
    print -e "error: failed to install devkit binary"
    exit 1
}

if $has_ci_agent {
    gum spin --spinner dot --title "Installing ci-agent..." -- go install $"github.com/89jobrien/devkit/cmd/ci-agent@($devkit_version)"
    if $env.LAST_EXIT_CODE != 0 {
        print -e "error: failed to install ci-agent binary"
        exit 1
    }
}

if $has_meta {
    gum spin --spinner dot --title "Installing meta..." -- go install $"github.com/89jobrien/devkit/cmd/meta@($devkit_version)"
    if $env.LAST_EXIT_CODE != 0 {
        print -e "error: failed to install meta binary"
        exit 1
    }
}

# --- build .devkit.toml ---

let ci_filtered = ($ci_platforms | where { |it| $it != "none" })
let ci_toml_array = if ($ci_filtered | is-empty) {
    "[]"
} else {
    let quoted = ($ci_filtered | each { |it| $"\"($it)\"" } | str join ", ")
    $"[($quoted)]"
}

let install_date = (date now | format date "%Y-%m-%d")

let safe_name   = (toml_escape $project_name)
let safe_desc   = (toml_escape $project_description)
let safe_focus  = (toml_escape $review_focus)

let toml_content = $"[project]
name        = \"($safe_name)\"
description = \"($safe_desc)\"
install_date = \"($install_date)\"
ci_platforms = ($ci_toml_array)

[components]
council  = ($has_council)
review   = ($has_review)
meta     = ($has_meta)
ci_agent = ($has_ci_agent)
diagnose = ($has_diagnose)

[review]
focus = \"($safe_focus)\"

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

# --- CI workflow files ---

def copy_ci_file [platform: string] {
    if $platform == "none" { return }
    let dest_dir = if $platform == "gitea" { ".gitea/workflows" } else { ".github/workflows" }
    mkdir $dest_dir
    let local_src = $"ci/($platform).yml"
    if ($local_src | path exists) {
        cp $local_src $"($dest_dir)/ci.yml"
        print $"Copied ($local_src) -> ($dest_dir)/ci.yml"
    } else if $have_curl {
        let url = $"https://raw.githubusercontent.com/89jobrien/devkit/($devkit_version)/ci/($platform).yml"
        let result = (do { run-external "curl" "-fsSL" $url "-o" $"($dest_dir)/ci.yml" } | complete)
        if $result.exit_code != 0 {
            print -e $"error: failed to download ($platform) CI template from ($url)"
            exit 1
        }
        print $"Downloaded ($platform) CI template -> ($dest_dir)/ci.yml"
    } else if $have_wget {
        let url = $"https://raw.githubusercontent.com/89jobrien/devkit/($devkit_version)/ci/($platform).yml"
        let result = (do { run-external "wget" "-qO" $"($dest_dir)/ci.yml" $url } | complete)
        if $result.exit_code != 0 {
            print -e $"error: failed to download ($platform) CI template (wget)"
            exit 1
        }
        print $"Downloaded ($platform) CI template -> ($dest_dir)/ci.yml"
    } else {
        print -e $"error: cannot download ($platform) CI template — neither curl nor wget found"
        print -e $"  Manually copy ci/($platform).yml to ($dest_dir)/ci.yml"
        exit 1
    }
}

for p in $ci_filtered {
    copy_ci_file $p
}

# --- git hooks ---

def install_hook [hook_path: string, content: string, label: string] {
    if ($hook_path | path exists) {
        print $"warning: ($hook_path) already exists — backing up to ($hook_path).bak"
        cp $hook_path $"($hook_path).bak"
    }
    $content | save -f $hook_path
    run-external "chmod" "+x" $hook_path
    print $"Installed ($label)"
}

if (".git" | path exists) {
    if $has_review {
        install_hook ".git/hooks/pre-commit" "#!/bin/sh
if [ \"${DEVKIT_SKIP_HOOKS:-0}\" = \"1\" ]; then
  exit 0
fi
devkit review --base HEAD
" "git pre-commit hook (devkit review)"
    }

    if $has_council {
        install_hook ".git/hooks/pre-push" "#!/bin/sh
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
" "git pre-push hook (devkit council)"
    }
} else {
    print "warning: not a git repository — skipping hook installation"
}

# --- Claude Code hooks ---

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
        let existing = (open $claude_settings)
        let new_entry = ($hook_json | from json | get hooks.PostToolUse | first)
        let existing_cmds = (
            $existing
            | get -i hooks.PostToolUse
            | default []
            | each { |e| $e | get -i hooks | default [] | each { |h| $h | get -i command | default "" } }
            | flatten
        )
        let new_cmd = ($new_entry | get hooks | first | get command)
        let merged = if ($existing_cmds | any { |c| $c == $new_cmd }) {
            $existing
        } else {
            let ptu = ($existing | get -i hooks.PostToolUse | default [] | append $new_entry)
            $existing | upsert hooks.PostToolUse $ptu
        }
        $merged | to json --indent 2 | save -f $claude_settings
        print $"Merged Claude Code hooks into ($claude_settings)"
    } else {
        $hook_json | save -f $claude_settings
        print $"Wrote Claude Code hooks to ($claude_settings)"
    }
}

# --- summary ---

print ""
gum style --bold "Installation complete."
print $"  Project:    ($project_name)"
print $"  Components: ($components | str join ', ')"
print $"  CI:         ($ci_filtered | str join ', ' | if ($in == '') { 'none' } else { $in })"
print $"  Config:     .devkit.toml"
