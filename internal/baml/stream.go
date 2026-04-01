package baml

import (
	"fmt"
	"io"
	"strings"
)

// renderStreamTokens drains a channel of partial token strings, writing each
// token to out as it arrives and returning the fully accumulated string.
func renderStreamTokens(ch <-chan string, out io.Writer) (string, error) {
	var sb strings.Builder
	for tok := range ch {
		if _, err := fmt.Fprint(out, tok); err != nil {
			return sb.String(), err
		}
		sb.WriteString(tok)
	}
	return sb.String(), nil
}
