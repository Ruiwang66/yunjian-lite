package shared

import (
	"strings"
	"testing"
)

func TestOutputCredentialRedaction(t *testing.T) {
	value := RedactOutput("Password: abc\napi_key=def\nAuthorization: Bearer ghj\nCPU 4%")
	for _, s := range []string{"abc", "def", "ghj"} {
		if strings.Contains(value, s) {
			t.Fatal("secret leaked: ", value)
		}
	}
	if !strings.Contains(value, "CPU 4%") {
		t.Fatal("normal output lost")
	}
}
