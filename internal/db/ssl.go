// TLS configuration for database drivers. Core defines the profile SSL modes.
package db

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/turanmahmudov/masume/internal/core"
)

// BuildPolicyTLS returns TLS settings for the policy and the certificate files, or nil for
// unset and disabled policies.
func BuildPolicyTLS(policy core.SSLPolicy, host string, files core.SSLFiles) (*tls.Config, error) {
	if policy == core.PolicyUnset || policy == core.PolicyOff {
		return nil, nil
	}
	if reason := core.FindSSLFilesProblem(files); reason != "" {
		return nil, NewDatabaseError("%s", reason)
	}
	certificates, err := loadClientCertificate(files)
	if err != nil {
		return nil, err
	}
	if !core.VerifiesCertificate(policy) {
		// Prefer and require encrypt without certificate verification.
		return &tls.Config{
			InsecureSkipVerify: true, MinVersion: tls.VersionTLS12,
			Certificates: certificates,
		}, nil
	}

	roots, err := loadRootCertificates(files.RootCert)
	if err != nil {
		return nil, err
	}
	if policy == core.PolicyVerifyFull {
		return &tls.Config{
			ServerName: host, MinVersion: tls.VersionTLS12,
			RootCAs: roots, Certificates: certificates,
		}, nil
	}
	return buildAuthorityOnlyTLS(roots, certificates), nil
}

// loadClientCertificate returns the client keypair of the files, or nothing for a set
// without one.
func loadClientCertificate(files core.SSLFiles) ([]tls.Certificate, error) {
	if !files.SendsClientCertificate() {
		return nil, nil
	}
	pair, err := tls.LoadX509KeyPair(
		core.ExpandHomePath(files.Cert), core.ExpandHomePath(files.Key))
	if err != nil {
		return nil, NewDatabaseError(
			"cannot read the client certificate %s with the key %s: %v",
			files.Cert, files.Key, err)
	}
	return []tls.Certificate{pair}, nil
}

// loadRootCertificates returns the authorities of that bundle, or nil for the system trust
// store.
func loadRootCertificates(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, nil
	}
	held, err := os.ReadFile(core.ExpandHomePath(path))
	if err != nil {
		return nil, NewDatabaseError("cannot read the certificate authority %s: %v", path, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(held) {
		return nil, NewDatabaseError("%s contains no PEM certificate", path)
	}
	return roots, nil
}

// buildAuthorityOnlyTLS verifies certificate chains against the roots without hostname
// verification. Nil roots use the system trust store.
func buildAuthorityOnlyTLS(roots *x509.CertPool, certificates []tls.Certificate) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		Certificates:       certificates,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			parsedCerts := make([]*x509.Certificate, 0, len(rawCerts))
			for _, raw := range rawCerts {
				parsed, err := x509.ParseCertificate(raw)
				if err != nil {
					return err
				}
				parsedCerts = append(parsedCerts, parsed)
			}
			if len(parsedCerts) == 0 {
				return NewDatabaseError("the server sent no certificate")
			}

			trusted := roots
			if trusted == nil {
				system, err := x509.SystemCertPool()
				if err != nil {
					return err
				}
				trusted = system
			}
			intermediates := x509.NewCertPool()
			for _, intermediate := range parsedCerts[1:] {
				intermediates.AddCert(intermediate)
			}
			_, err := parsedCerts[0].Verify(x509.VerifyOptions{
				Roots: trusted, Intermediates: intermediates,
			})
			return err
		},
	}
}
