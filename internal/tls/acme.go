package tls

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/route53"
)

// ACMEConfig holds configuration for ACME certificate management.
type ACMEConfig struct {
	Domain  string
	Email   string
	Staging bool
	DataDir string
}

// NewACMETLS configures CertMagic with Route53 DNS-01 and obtains/renews
// a certificate for the configured domain. Returns a tls.Config with
// GetCertificate wired up to CertMagic.
//
// AWS credentials are loaded from the standard environment variables
// (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION) or IAM role.
func NewACMETLS(ctx context.Context, cfg ACMEConfig) (*tls.Config, error) {
	storage := &certmagic.FileStorage{
		Path: filepath.Join(cfg.DataDir, "certmagic"),
	}

	magic := certmagic.NewDefault()
	magic.Storage = storage

	ca := certmagic.LetsEncryptProductionCA
	if cfg.Staging {
		ca = certmagic.LetsEncryptStagingCA
	}

	issuer := certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
		CA:     ca,
		Email:  cfg.Email,
		Agreed: true,
		DNS01Solver: &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &route53.Provider{},
				// Use public DNS for SOA-based zone detection so split-horizon
				// local DNS doesn't cause CertMagic to pick the wrong zone.
				Resolvers: []string{"8.8.8.8:53", "1.1.1.1:53"},
				// Wait for DNS propagation before asking LE to verify.
				PropagationDelay:   30 * time.Second,
				PropagationTimeout: 2 * time.Minute,
			},
		},
	})
	magic.Issuers = []certmagic.Issuer{issuer}

	log.Printf("tls: obtaining ACME certificate for %s (staging=%v)", cfg.Domain, cfg.Staging)

	if err := magic.ManageSync(ctx, []string{cfg.Domain}); err != nil {
		return nil, fmt.Errorf("certmagic manage: %w", err)
	}

	log.Printf("tls: ACME certificate ready for %s", cfg.Domain)

	tlsCfg := magic.TLSConfig()
	tlsCfg.NextProtos = []string{"h2", "http/1.1"}
	tlsCfg.MinVersion = tls.VersionTLS12
	return tlsCfg, nil
}
