package bitbucket

import (
	"testing"

	"prdash/internal/testutil"
)

// TestConformance comprueba que el adapter inerte cumple el contrato y responde
// "no soportado" en todo, sin realizar ninguna llamada de red.
func TestConformance(t *testing.T) {
	testutil.RunConformance(t, New("bitbucket.org"), testutil.ConformanceOptions{Unsupported: true})
}
