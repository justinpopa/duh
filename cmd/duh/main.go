package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/justinpopa/duh/internal/config"
	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/httpserver"
	"github.com/justinpopa/duh/internal/proxydhcp"
	"github.com/justinpopa/duh/internal/tftpserver"
	duhtls "github.com/justinpopa/duh/internal/tls"
	"github.com/justinpopa/duh/web"
)

var version = "dev"

func main() {
	cfg := config.Parse()

	if cfg.Version {
		fmt.Println("duh " + version)
		os.Exit(0)
	}

	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	tmplFS, err := fs.Sub(web.TemplatesFS, "templates")
	if err != nil {
		log.Fatalf("templates fs: %v", err)
	}
	statFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	srv, err := httpserver.New(database, cfg.DataDir, cfg.ServerURL, cfg.CatalogURL, cfg.TFTPAddr, cfg.HTTPAddr, cfg.ProxyDHCP, tmplFS, statFS)
	if err != nil {
		log.Fatalf("http server: %v", err)
	}
	defer srv.Webhook.Close()

	handler := srv.Handler()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	// TFTP server
	tftpSrv := tftpserver.NewServer(cfg.TFTPAddr)
	g.Go(func() error {
		log.Printf("tftp: listening on %s", cfg.TFTPAddr)

		ln, err := net.ListenPacket("udp", cfg.TFTPAddr)
		if err != nil {
			return err
		}

		go func() {
			<-ctx.Done()
			tftpSrv.Shutdown()
		}()

		return tftpSrv.Serve(ln.(*net.UDPConn))
	})

	// HTTP server
	g.Go(func() error {
		httpHandler := handler
		if cfg.HTTPSRedirect {
			// Extract port from HTTPS address for redirect target
			httpsPort := "443"
			if _, p, err := net.SplitHostPort(cfg.HTTPSAddr); err == nil {
				httpsPort = p
			}
			httpHandler = httpserver.HTTPSRedirectMiddleware(httpsPort, handler)
			log.Print("http: HTTPS redirect enabled (iPXE clients excluded)")
		}

		httpSrv := &http.Server{
			Addr:    cfg.HTTPAddr,
			Handler: httpHandler,
		}
		log.Printf("http: listening on %s", cfg.HTTPAddr)

		go func() {
			<-ctx.Done()
			httpSrv.Close()
		}()

		if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	// HTTPS server
	g.Go(func() error {
		tlsCfg, err := duhtls.ProvideTLS(ctx, duhtls.Options{
			DataDir:     cfg.DataDir,
			CertFile:    cfg.TLSCertFile,
			KeyFile:     cfg.TLSKeyFile,
			ACMEDomain:  cfg.ACMEDomain,
			ACMEEmail:   cfg.ACMEEmail,
			ACMEStaging: cfg.ACMEStaging,
		})
		if err != nil {
			log.Printf("tls: %v (HTTPS disabled)", err)
			return nil
		}

		httpsSrv := &http.Server{
			Addr:      cfg.HTTPSAddr,
			Handler:   handler,
			TLSConfig: tlsCfg,
		}
		log.Printf("https: listening on %s", cfg.HTTPSAddr)

		go func() {
			<-ctx.Done()
			httpsSrv.Close()
		}()

		ln, err := tls.Listen("tcp", cfg.HTTPSAddr, tlsCfg)
		if err != nil {
			return err
		}

		if err := httpsSrv.Serve(ln); err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	// Proxy DHCP server (optional)
	if cfg.ProxyDHCP {
		g.Go(func() error {
			var serverIP net.IP
			iface := cfg.DHCPIface

			if iface == "" {
				detectedIface, detectedIP, err := proxydhcp.DetectInterface()
				if err != nil {
					return fmt.Errorf("proxy dhcp: %w", err)
				}
				iface = detectedIface
				serverIP = detectedIP
			} else {
				ip, err := proxydhcp.InterfaceIP(iface)
				if err != nil {
					return fmt.Errorf("proxy dhcp: %w", err)
				}
				serverIP = ip
			}

			log.Printf("proxydhcp: server IP %s on %s", serverIP, iface)

			pdhcp := proxydhcp.New(serverIP, cfg.TFTPAddr, cfg.HTTPAddr, cfg.ServerURL, iface)
			return pdhcp.ListenAndServe(ctx)
		})
	}

	if err := g.Wait(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
