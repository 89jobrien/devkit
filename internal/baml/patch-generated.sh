#!/usr/bin/env python3
# patch-generated.sh — fixes import bugs in baml-cli generated Go code.
# Run after: baml-cli generate
# These are known codegen bugs in baml-cli 0.218.x Go output.
import os, re, sys

BASE = os.path.join(os.path.dirname(__file__), "baml_client", "baml_client")

def read(path):
    with open(path) as f:
        return f.read()

def write(path, content):
    with open(path, "w") as f:
        f.write(content)

def blank_import(content, pkg):
    """Replace a named or unnamed import of pkg with a blank import."""
    # handles tab or space indentation, with or without alias
    return re.sub(
        r'([ \t]+)(\w+ )?"' + re.escape(pkg) + r'"',
        r'\1_ "' + pkg + r'"',
        content
    )

def add_import_after_paren(content, new_import):
    """Insert a new import line after the first `import (` line."""
    return content.replace('import (\n', 'import (\n\t' + new_import + '\n', 1)

def add_import_after_line(content, after_line, new_import):
    """Insert new_import on the line after after_line."""
    return content.replace(after_line, after_line + '\n\t' + new_import, 1)

# --- type_builder/enums.go has a single-line import (no parens) ---
path = os.path.join(BASE, "type_builder/enums.go")
c = read(path)
c = re.sub(r'import \w+ "github\.com/boundaryml/baml/engine/language_client_go/pkg"',
           'import _ "github.com/boundaryml/baml/engine/language_client_go/pkg"', c)
write(path, c)

# --- Empty stub files: blank all their imports ---
stub_files = [
    "types/enums.go",
    "types/type_aliases.go",
    "types/unions.go",
    "stream_types/type_aliases.go",
    "stream_types/unions.go",
    "stream_types/utils.go",
]
for rel in stub_files:
    path = os.path.join(BASE, rel)
    c = read(path)
    for pkg in [
        "encoding/json", "fmt",
        "github.com/boundaryml/baml/engine/language_client_go/pkg",
        "github.com/boundaryml/baml/engine/language_client_go/pkg/cffi",
        "baml_client/baml_client/types",
    ]:
        c = blank_import(c, pkg)
    write(path, c)

# --- types/classes.go: json not used ---
path = os.path.join(BASE, "types/classes.go")
c = blank_import(read(path), "encoding/json")
write(path, c)

# --- stream_types/classes.go: json and types not used ---
path = os.path.join(BASE, "stream_types/classes.go")
c = read(path)
c = blank_import(c, "encoding/json")
c = blank_import(c, "baml_client/baml_client/types")
write(path, c)

# --- type_builder/type_builder.go: missing fmt (single-line import, convert to block) ---
path = os.path.join(BASE, "type_builder/type_builder.go")
c = read(path)
if '"fmt"' not in c:
    c = re.sub(
        r'import (\w+) "github\.com/boundaryml/baml/engine/language_client_go/pkg"',
        'import (\n\t"fmt"\n\n\t\\1 "github.com/boundaryml/baml/engine/language_client_go/pkg"\n)',
        c, count=1
    )
write(path, c)

# --- runtime.go: missing fmt and type_builder imports ---
path = os.path.join(BASE, "runtime.go")
c = read(path)
if '"fmt"' not in c:
    c = add_import_after_paren(c, '"fmt"')
tb_import = '"baml_client/baml_client/type_builder"'
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
for rel in ["functions.go", "functions_parse.go", "functions_parse_stream.go", "functions_stream.go"]:
    path = os.path.join(BASE, rel)
    c = read(path)
    if '"fmt"' not in c:
        c = add_import_after_paren(c, '"fmt"')
    write(path, c)

# --- functions_parse.go: stream_types unused ---
path = os.path.join(BASE, "functions_parse.go")
c = blank_import(read(path), "baml_client/baml_client/stream_types")
write(path, c)

# --- functions_parse_stream.go: types unused ---
path = os.path.join(BASE, "functions_parse_stream.go")
c = blank_import(read(path), "baml_client/baml_client/types")
write(path, c)

print("patch-generated.sh: done")
