package db

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// writeCertificateFiles writes a self-signed authority and a client keypair, and returns
// their paths.
func writeCertificateFiles(t *testing.T) core.SSLFiles {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("the key was not built: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "masume test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	written, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("the certificate was not built: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("the key was not written: %v", err)
	}

	directory := t.TempDir()
	files := core.SSLFiles{
		RootCert: filepath.Join(directory, "root.pem"),
		Cert:     filepath.Join(directory, "client.pem"),
		Key:      filepath.Join(directory, "client.key"),
	}
	for path, block := range map[string]*pem.Block{
		files.RootCert: {Type: "CERTIFICATE", Bytes: written},
		files.Cert:     {Type: "CERTIFICATE", Bytes: written},
		files.Key:      {Type: "EC PRIVATE KEY", Bytes: keyBytes},
	} {
		if writeErr := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); writeErr != nil {
			t.Fatalf("%s was not written: %v", path, writeErr)
		}
	}
	return files
}

// A managed database ships its own authority. `verify-full` checks the chain against that
// bundle and not against the system trust store.
func TestBuildPolicyTLSVerifiesAgainstTheGivenAuthority(t *testing.T) {
	files := writeCertificateFiles(t)
	config, err := BuildPolicyTLS(
		core.PolicyVerifyFull, "db.example", core.SSLFiles{RootCert: files.RootCert})
	if err != nil {
		t.Fatalf("the settings were refused: %v", err)
	}
	if config.RootCAs == nil {
		t.Error("the connection uses the system trust store")
	}
	if config.ServerName != "db.example" || config.InsecureSkipVerify {
		t.Errorf("the host name is not checked: %+v", config)
	}
}

// `verify-ca` checks the chain itself, and it checks it against the bundle of the profile.
func TestBuildPolicyTLSVerifiesTheChainWithoutTheHostName(t *testing.T) {
	files := writeCertificateFiles(t)
	config, err := BuildPolicyTLS(
		core.PolicyVerifyCa, "db.example", core.SSLFiles{RootCert: files.RootCert})
	if err != nil {
		t.Fatalf("the settings were refused: %v", err)
	}
	if config.VerifyPeerCertificate == nil {
		t.Fatal("the certificate is not checked")
	}
	if config.ServerName != "" {
		t.Errorf("the host name is checked: %q", config.ServerName)
	}
}

// Mutual TLS sends the client certificate under every policy that encrypts.
func TestBuildPolicyTLSSendsTheClientCertificate(t *testing.T) {
	files := writeCertificateFiles(t)
	for _, policy := range []core.SSLPolicy{
		core.PolicyEncryptOnly, core.PolicyVerifyCa, core.PolicyVerifyFull,
	} {
		config, err := BuildPolicyTLS(policy, "db.example", files)
		if err != nil {
			t.Fatalf("%s was refused: %v", policy, err)
		}
		if len(config.Certificates) != 1 {
			t.Errorf("%s sends %d certificates, wanted 1", policy, len(config.Certificates))
		}
	}
}

// A certificate the client cannot read stops the connection, and the message names the
// path.
func TestBuildPolicyTLSReportsAnUnreadableFile(t *testing.T) {
	for _, held := range []struct {
		files  core.SSLFiles
		wanted string
	}{
		{core.SSLFiles{RootCert: "/held/absent.pem"}, "/held/absent.pem"},
		{core.SSLFiles{Cert: "/held/client.pem", Key: "/held/client.key"}, "/held/client.pem"},
		{core.SSLFiles{Cert: "/held/client.pem"}, "sslcert and sslkey must both be set"},
	} {
		_, err := BuildPolicyTLS(core.PolicyVerifyFull, "db.example", held.files)
		if err == nil {
			t.Fatalf("%+v was accepted", held.files)
		}
		if !strings.Contains(err.Error(), held.wanted) {
			t.Errorf("the error reads %q, wanted %q in it", err, held.wanted)
		}
	}
}

// A bundle without a certificate in it is refused. A wrong path is reported and never
// used as an empty trust store.
func TestBuildPolicyTLSRefusesABundleWithoutACertificate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root.pem")
	if err := os.WriteFile(path, []byte("held"), 0o600); err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
	if _, err := BuildPolicyTLS(
		core.PolicyVerifyCa, "db.example", core.SSLFiles{RootCert: path}); err == nil {
		t.Error("a bundle without a certificate was accepted")
	}
}

// A disabled policy carries no TLS, whatever files the profile holds.
func TestBuildPolicyTLSCarriesNoTLSWhereThePolicyIsOff(t *testing.T) {
	files := writeCertificateFiles(t)
	for _, policy := range []core.SSLPolicy{core.PolicyOff, core.PolicyUnset} {
		config, err := BuildPolicyTLS(policy, "db.example", files)
		if err != nil || config != nil {
			t.Errorf("%s answered %+v, %v", policy, config, err)
		}
	}
}
