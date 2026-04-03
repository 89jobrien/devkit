package baml

import (
	"bytes"
	"testing"
)

func TestRenderStreamAccumulatesTokens(t *testing.T) {
	var buf bytes.Buffer
	tokens := []string{"Hello", " world", "!"}
	ch := make(chan string, len(tokens))
	for _, tok := range tokens {
		ch <- tok
	}
	close(ch)

	result, err := renderStreamTokens(ch, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello world!" {
		t.Errorf("got %q, want %q", result, "Hello world!")
	}
	if buf.String() != "Hello world!" {
		t.Errorf("buf got %q, want %q", buf.String(), "Hello world!")
	}
}

func TestRenderStreamEmpty(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan string)
	close(ch)

	result, err := renderStreamTokens(ch, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}
