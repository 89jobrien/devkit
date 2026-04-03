Fill in every section of this template exactly. Do not add or remove sections.

Output ONLY valid Go test code. No prose, no markdown outside this block.

```go
package <pkg>_test

import (
    "testing"
    // add imports as needed
)

// TestXxx tests the Xxx function/method.
// Use table-driven tests when multiple inputs are needed.
func TestXxx(t *testing.T) {
    tests := []struct {
        name  string
        input <type>
        want  <type>
    }{
        {"case name", <input>, <expected>},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Xxx(tt.input)
            if got != tt.want {
                t.Errorf("Xxx(%v) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}

// Add one TestXxx function per exported symbol.
// Replace TODO bodies with actual assertions.
```
