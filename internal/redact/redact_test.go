package redact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestT5_RedactorMasksSecrets(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Add("0123456789abcdef0123456789abcdef", "short", "")
	fmt.Fprintf(w, "calling radarr with key 0123456789abcdef0123456789abcdef twice 0123456789abcdef0123456789abcdef\n")
	fmt.Fprintf(w, "short words stay\n")
	out := buf.String()
	if strings.Contains(out, "0123456789abcdef") {
		t.Fatalf("secret leaked: %q", out)
	}
	if strings.Count(out, Mask) != 2 {
		t.Fatalf("want 2 masks, got %q", out)
	}
	if !strings.Contains(out, "short words stay") {
		t.Fatal("values under 8 chars must not be masked")
	}
}
