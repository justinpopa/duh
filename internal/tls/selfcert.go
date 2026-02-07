package tls

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// discoverSANs enumerates all non-loopback interface IPs, the system hostname,
// and "localhost" to build a complete set of SANs for a self-signed certificate.
func discoverSANs() (dnsNames []string, ips []net.IP) {
	dnsSet := map[string]bool{"localhost": true}

	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		hostname = strings.ToLower(hostname)
		dnsSet[hostname] = true

		// Build FQDN from hostname + search domain in /etc/resolv.conf
		for _, search := range readSearchDomains() {
			dnsSet[hostname+"."+search] = true
		}
	}

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() {
					ips = append(ips, ip)
				}
			}
		}
	}

	// Always include loopback IPs
	ips = append(ips, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))

	for name := range dnsSet {
		dnsNames = append(dnsNames, name)
	}
	sort.Strings(dnsNames)
	sort.Slice(ips, func(i, j int) bool {
		return ips[i].String() < ips[j].String()
	})

	return dnsNames, ips
}

// readSearchDomains parses /etc/resolv.conf for "search" and "domain" directives.
func readSearchDomains() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer f.Close()

	var domains []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "search":
			domains = append(domains, fields[1:]...)
		case "domain":
			domains = append(domains, fields[1])
		}
	}
	return domains
}

// sansMatch checks whether an existing certificate covers the same SANs
// as the currently discovered ones. Returns true if they match.
func sansMatch(cert *x509.Certificate, wantDNS []string, wantIPs []net.IP) bool {
	haveDNS := make([]string, len(cert.DNSNames))
	copy(haveDNS, cert.DNSNames)
	sort.Strings(haveDNS)

	if !slices.Equal(haveDNS, wantDNS) {
		return false
	}

	haveIPs := make([]string, len(cert.IPAddresses))
	for i, ip := range cert.IPAddresses {
		haveIPs[i] = ip.String()
	}
	sort.Strings(haveIPs)

	wantIPStrs := make([]string, len(wantIPs))
	for i, ip := range wantIPs {
		wantIPStrs[i] = ip.String()
	}
	sort.Strings(wantIPStrs)

	return slices.Equal(haveIPs, wantIPStrs)
}

// loadAndCheckSelfSigned loads an existing self-signed cert and checks whether
// it is still valid (not expired, SANs match). Returns the tls.Config if valid,
// or nil if the cert should be regenerated.
func loadAndCheckSelfSigned(certPath, keyPath string, wantDNS []string, wantIPs []net.IP) *tls.Config {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	// Check expiry (regenerate if less than 30 days remaining)
	if time.Until(x509Cert.NotAfter) < 30*24*time.Hour {
		log.Print("tls: self-signed cert expiring soon, regenerating")
		return nil
	}

	if !sansMatch(x509Cert, wantDNS, wantIPs) {
		log.Print("tls: SANs changed, regenerating self-signed cert")
		return nil
	}

	tlsCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}
}

// LoadOrGenerateSelfSigned returns a TLS config using a self-signed certificate
// with SANs covering all local interfaces, the hostname, and localhost.
// If an existing cert at the standard path is still valid and has matching SANs,
// it is reused.
func LoadOrGenerateSelfSigned(dataDir string) (*tls.Config, error) {
	certPath := filepath.Join(dataDir, "tls", "cert.pem")
	keyPath := filepath.Join(dataDir, "tls", "key.pem")

	dnsNames, ipAddrs := discoverSANs()

	log.Printf("tls: discovered SANs — DNS: %v, IPs: %v", dnsNames, ipAddrs)

	if cfg := loadAndCheckSelfSigned(certPath, keyPath, dnsNames, ipAddrs); cfg != nil {
		log.Print("tls: reusing existing self-signed cert")
		return cfg, nil
	}

	log.Print("tls: generating new self-signed cert")
	return generateSelfSigned(certPath, keyPath, dnsNames, ipAddrs)
}

func generateSelfSigned(certPath, keyPath string, dnsNames []string, ipAddrs []net.IP) (*tls.Config, error) {
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return nil, fmt.Errorf("create TLS dir: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "duh"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  ipAddrs,
		DNSNames:     dnsNames,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse generated keypair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
