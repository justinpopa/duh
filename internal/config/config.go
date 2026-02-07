package config

import (
	"flag"
	"os"
)

type Config struct {
	Version       bool
	DataDir       string
	TFTPAddr      string
	HTTPAddr      string
	HTTPSAddr     string
	TLSCertFile   string
	TLSKeyFile    string
	ACMEDomain    string
	ACMEEmail     string
	ACMEStaging   bool
	HTTPSRedirect bool
	ServerURL     string
	CatalogURL    string
	ProxyDHCP     bool
	DHCPIface     string
}

func Parse() *Config {
	c := &Config{}

	flag.BoolVar(&c.Version, "version", false, "print version and exit")
	flag.StringVar(&c.DataDir, "data-dir", envOr("DUH_DATA_DIR", "./data"), "data directory")
	flag.StringVar(&c.TFTPAddr, "tftp-addr", envOr("DUH_TFTP_ADDR", ":69"), "TFTP listen address")
	flag.StringVar(&c.HTTPAddr, "http-addr", envOr("DUH_HTTP_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&c.HTTPSAddr, "https-addr", envOr("DUH_HTTPS_ADDR", ":8443"), "HTTPS listen address")
	flag.StringVar(&c.TLSCertFile, "tls-cert", envOr("DUH_TLS_CERT", ""), "TLS certificate file (auto-generate if empty)")
	flag.StringVar(&c.TLSKeyFile, "tls-key", envOr("DUH_TLS_KEY", ""), "TLS key file (auto-generate if empty)")
	flag.StringVar(&c.ACMEDomain, "acme-domain", envOr("DUH_ACME_DOMAIN", ""), "domain for ACME/Let's Encrypt certificate")
	flag.StringVar(&c.ACMEEmail, "acme-email", envOr("DUH_ACME_EMAIL", ""), "email for ACME account registration")
	flag.BoolVar(&c.ACMEStaging, "acme-staging", envOr("DUH_ACME_STAGING", "") != "", "use Let's Encrypt staging CA")
	flag.BoolVar(&c.HTTPSRedirect, "https-redirect", envOr("DUH_HTTPS_REDIRECT", "") != "", "redirect HTTP to HTTPS (iPXE clients excluded)")
	flag.StringVar(&c.ServerURL, "server-url", envOr("DUH_SERVER_URL", ""), "server URL for iPXE scripts (auto-detect if empty)")
	flag.StringVar(&c.CatalogURL, "catalog-url", envOr("DUH_CATALOG_URL", "https://raw.githubusercontent.com/justinpopa/duh-catalog/main/catalog.json"), "image catalog URL")
	flag.BoolVar(&c.ProxyDHCP, "proxy-dhcp", envOr("DUH_PROXY_DHCP", "") != "", "enable proxy DHCP server for PXE")
	flag.StringVar(&c.DHCPIface, "dhcp-iface", envOr("DUH_DHCP_IFACE", ""), "network interface for proxy DHCP (auto-detect if empty)")

	flag.Parse()
	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
