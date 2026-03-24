// internal/meta/meta_test.go
package meta_test

import (
	"context"
	"testing"

	"github.com/89jobrien/devkit/internal/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct {
	designResponse string
	workerResponse string
}

func (s *stubRunner) Run(_ context.Context, prompt string, _ []string) (string, error) {
	if len(s.designResponse) > 0 && s.designResponse != "used" {
		r := s.designResponse
		s.designResponse = "used"
		return r, nil
	}
	return s.workerResponse, nil
}

func TestRunDesignsAndExecutes(t *testing.T) {
	runner := &stubRunner{
		designResponse: `[{"name":"checker","role":"check stuff","prompt":"check the code","tools":["Read"]}]`,
		workerResponse: "found 2 issues",
	}
	result, err := meta.Run(context.Background(), "audit the code", "repo context", "sdk docs", runner)
	require.NoError(t, err)
	require.Len(t, result.Plan, 1)
	assert.Equal(t, "checker", result.Plan[0].Name)
	assert.Contains(t, result.Outputs["checker"], "found 2 issues")
}

func TestRunFallsBackOnInvalidJSON(t *testing.T) {
	runner := &stubRunner{
		designResponse: "not valid json",
		workerResponse: "analysis complete",
	}
	result, err := meta.Run(context.Background(), "do something", "", "", runner)
	require.NoError(t, err)
	assert.Len(t, result.Plan, 1)
	assert.Equal(t, "analyst", result.Plan[0].Name)
}
