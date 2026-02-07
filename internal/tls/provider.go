package tls

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
)

// Options holds all TLS-related configuration.
type Options struct {
	DataDir    string
	CertFile   string
	KeyFile    string
	ACMEDomain string
	ACMEEmail  string
	ACMEStaging bool
}

// ProvideTLS returns a *tls.Config based on the following decision tree:
//  1. ACME domain set → obtain cert via CertMagic with Route53 DNS-01
//  2. Cert + key files provided → load user-supplied keypair
//  3. Otherwise → self-signed with auto-discovered SANs
func ProvideTLS(ctx context.Context, opts Options) (*tls.Config, error) {
	if opts.ACMEDomain != "" {
		log.Print("tls: using ACME/CertMagic provider")
		return NewACMETLS(ctx, ACMEConfig{
			Domain:  opts.ACMEDomain,
			Email:   opts.ACMEEmail,
			Staging: opts.ACMEStaging,
			DataDir: opts.DataDir,
		})
	}

	if opts.CertFile != "" && opts.KeyFile != "" {
		log.Print("tls: using user-provided certificate")
		cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load TLS keypair: %w", err)
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	log.Print("tls: using self-signed certificate")
	return LoadOrGenerateSelfSigned(opts.DataDir)
}
