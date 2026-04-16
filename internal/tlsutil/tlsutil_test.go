package tlsutil_test

import (
	"crypto/tls"
	"path/filepath"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/tlsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureCerts_GeneratesOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	err := tlsutil.EnsureCerts(dir)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "server.crt"))
	assert.FileExists(t, filepath.Join(dir, "server.key"))
}

func TestEnsureCerts_LoadsExistingCert(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, tlsutil.EnsureCerts(dir))
	// Second call must not overwrite
	require.NoError(t, tlsutil.EnsureCerts(dir))
	_, err := tls.LoadX509KeyPair(
		filepath.Join(dir, "server.crt"),
		filepath.Join(dir, "server.key"))
	assert.NoError(t, err)
}
