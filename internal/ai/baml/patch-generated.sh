#!/usr/bin/env python3
# patch-generated.sh — fixes import bugs in baml-cli generated Go code.
# Run after: baml-cli generate
# These are known codegen bugs in baml-cli 0.220.x Go output.
# Layout change in 0.220.0: generated files are now at baml_client/ root
# (was nested baml_client/baml_client/ in 0.218.x).
import os, re, sys

BASE = os.path.join(os.path.dirname(__file__), "baml_client")


def read(path):
    with open(path) as f:
        return f.read()


def write(path, content):
    with open(path, "w") as f:
        f.write(content)


def blank_import(content, pkg):
    """Replace a named or unnamed import of pkg with a blank import."""
    return re.sub(
        r'([ \t]+)(\w+ )?"' + re.escape(pkg) + r'"',
        r'\1_ "' + pkg + r'"',
        content,
    )


def add_import_after_paren(content, new_import):
    """Insert a new import line after the first `import (` line."""
    return content.replace("import (\n", "import (\n\t" + new_import + "\n", 1)


def add_import_after_line(content, after_line, new_import):
    """Insert new_import on the line after after_line."""
    return content.replace(after_line, after_line + "\n\t" + new_import, 1)


# Fix baml_devkit/baml_client/* -> baml_devkit/* (codegen bug: wrong module-relative path)
all_go = []
for dirpath, _, filenames in os.walk(BASE):
    for fn in filenames:
        if fn.endswith(".go"):
            all_go.append(os.path.join(dirpath, fn))

for path in all_go:
    c = read(path)
    patched = c.replace("baml_devkit/baml_client/", "baml_devkit/")
    if patched != c:
        write(path, patched)

# --- Stub files: blank all unused imports ---
# types/enums.go is only a stub when it has no func definitions (empty enum list).
# When enums are defined, the imports are actually used.
stub_files = [
    "types/type_aliases.go",
    "types/unions.go",
    "stream_types/type_aliases.go",
    "stream_types/unions.go",
    "stream_types/utils.go",
]
enums_path = os.path.join(BASE, "types/enums.go")
enums_content = read(enums_path)
if "\nfunc " not in enums_content:
    # Stub: no functions defined, blank unused imports
    stub_files.append("types/enums.go")

for rel in stub_files:
    path = os.path.join(BASE, rel)
    c = read(path)
    for pkg in [
        "encoding/json",
        "fmt",
        "github.com/boundaryml/baml/engine/language_client_go/pkg",
        "github.com/boundaryml/baml/engine/language_client_go/pkg/cffi",
        "baml_devkit/types",
    ]:
        c = blank_import(c, pkg)
    write(path, c)

# --- types/classes.go: json not used ---
path = os.path.join(BASE, "types/classes.go")
c = blank_import(read(path), "encoding/json")
write(path, c)

# --- stream_types/classes.go: json not used; types only blanked if not referenced ---
path = os.path.join(BASE, "stream_types/classes.go")
c = read(path)
c = blank_import(c, "encoding/json")
# Only blank types import if file does not reference the types package via `types.`
if "types." not in c:
    c = blank_import(c, "baml_devkit/types")
write(path, c)

# --- type_builder/enums.go: only blank baml import if file doesn't use it ---
path = os.path.join(BASE, "type_builder/enums.go")
c = read(path)
# If the file references baml. directly (enum builders etc), keep the import real.
if "baml." not in c:
    c = re.sub(
        r'import \w+ "github\.com/boundaryml/baml/engine/language_client_go/pkg"',
        'import _ "github.com/boundaryml/baml/engine/language_client_go/pkg"',
        c,
    )
write(path, c)

# --- type_builder/type_builder.go: missing fmt ---
path = os.path.join(BASE, "type_builder/type_builder.go")
c = read(path)
if '"fmt"' not in c:
    c = re.sub(
        r'import (\w+) "github\.com/boundaryml/baml/engine/language_client_go/pkg"',
        'import (\n\t"fmt"\n\n\t\\1 "github.com/boundaryml/baml/engine/language_client_go/pkg"\n)',
        c,
        count=1,
    )
write(path, c)

# --- runtime.go: missing fmt and type_builder imports ---
path = os.path.join(BASE, "runtime.go")
c = read(path)
if '"fmt"' not in c:
    c = add_import_after_paren(c, '"fmt"')
tb_import = '"baml_devkit/type_builder"'
if tb_import not in c:
    anchor = '\tbaml "github.com/boundaryml/baml/engine/language_client_go/pkg"'
    c = add_import_after_line(c, anchor, tb_import)
write(path, c)

# --- type_map.go: missing reflect; baml unused ---
path = os.path.join(BASE, "type_map.go")
c = read(path)
if '"reflect"' not in c:
    c = add_import_after_paren(c, '"reflect"')
c = blank_import(c, "github.com/boundaryml/baml/engine/language_client_go/pkg")
write(path, c)

# --- functions*.go: missing fmt ---
for rel in [
    "functions.go",
    "functions_parse.go",
    "functions_parse_stream.go",
    "functions_stream.go",
    "functions_build_request.go",
    "functions_build_request_stream.go",
]:
    path = os.path.join(BASE, rel)
    c = read(path)
    if '"fmt"' not in c:
        c = add_import_after_paren(c, '"fmt"')
    write(path, c)

# --- functions_parse.go: stream_types unused ---
path = os.path.join(BASE, "functions_parse.go")
c = blank_import(read(path), "baml_devkit/stream_types")
write(path, c)

# --- functions_parse_stream.go: types unused ---
path = os.path.join(BASE, "functions_parse_stream.go")
c = blank_import(read(path), "baml_devkit/types")
write(path, c)

# --- functions_build_request*.go: types unused ---
for rel in ["functions_build_request.go", "functions_build_request_stream.go"]:
    path = os.path.join(BASE, rel)
    c = blank_import(read(path), "baml_devkit/types")
    write(path, c)

print("patch-generated.sh: done")
