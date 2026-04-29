package tunnel_test

import (
	"crypto/tls"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestGenerateCA(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)
	assert.NotNil(t, ca.Certificate)
	assert.NotNil(t, ca.PrivateKey)
	assert.NotEmpty(t, ca.CertPEM)
}

func TestSignCert(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)

	cert, err := tunnel.SignCert(ca, "agent-node-1")
	assert.NoError(t, err)
	assert.NotEmpty(t, cert.CertPEM)
	assert.NotEmpty(t, cert.KeyPEM)

	_, err = tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	assert.NoError(t, err)
}

func TestServerTLSConfig(t *testing.T) {
	ca, _ := tunnel.GenerateCA()
	serverCert, _ := tunnel.SignCert(ca, "o3k-server")

	cfg, err := tunnel.ServerTLSConfig(ca, serverCert)
	assert.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
}

func TestClientTLSConfig(t *testing.T) {
	ca, _ := tunnel.GenerateCA()
	clientCert, _ := tunnel.SignCert(ca, "agent-1")

	cfg, err := tunnel.ClientTLSConfig(ca, clientCert)
	assert.NoError(t, err)
	assert.Len(t, cfg.Certificates, 1)
	assert.NotNil(t, cfg.RootCAs)
}
