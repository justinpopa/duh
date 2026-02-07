package proxydhcp

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/insomniacslk/dhcp/iana"
)

// Server is a proxy DHCP server that responds to PXE clients with boot info
// without assigning IP addresses. It works alongside an existing DHCP server.
type Server struct {
	ServerIP  net.IP
	TFTPAddr  string
	HTTPAddr  string
	ServerURL string
	iface     string
}

func New(serverIP net.IP, tftpAddr, httpAddr, serverURL, iface string) *Server {
	return &Server{
		ServerIP:  serverIP,
		TFTPAddr:  tftpAddr,
		HTTPAddr:  httpAddr,
		ServerURL: serverURL,
		iface:     iface,
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	laddr := &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 67}

	srv, err := server4.NewServer(s.iface, laddr, s.handler)
	if err != nil {
		// If port 67 fails (e.g. another DHCP server on this host), try port 4011
		laddr.Port = 4011
		srv, err = server4.NewServer(s.iface, laddr, s.handler)
		if err != nil {
			return fmt.Errorf("proxy dhcp: %w", err)
		}
	}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	log.Printf("proxydhcp: listening on %s port %d", s.iface, laddr.Port)
	return srv.Serve()
}

func (s *Server) handler(conn net.PacketConn, peer net.Addr, pkt *dhcpv4.DHCPv4) {
	// Only respond to DHCP DISCOVERs and REQUESTs from PXE clients
	if pkt.MessageType() != dhcpv4.MessageTypeDiscover && pkt.MessageType() != dhcpv4.MessageTypeRequest {
		return
	}

	// Check for PXEClient or HTTPClient vendor class (option 60)
	httpBoot := isHTTPBootClient(pkt)
	if !isPXEClient(pkt) && !httpBoot {
		return
	}

	// Detect if this is an iPXE client (user-class option 77)
	isIPXE := isIPXEClient(pkt)

	// Get client architecture from option 93
	arch := clientArch(pkt)

	method := "pxe"
	if httpBoot {
		method = "http"
	}
	log.Printf("proxydhcp: %s from %s arch=%s ipxe=%v method=%s",
		pkt.MessageType(), pkt.ClientHWAddr, archName(arch), isIPXE, method)

	serverURL := s.ServerURL
	if serverURL == "" {
		serverURL = fmt.Sprintf("http://%s%s", s.ServerIP, s.HTTPAddr)
	}

	var bootFile string
	if isIPXE {
		// iPXE is loaded - chain to our boot script
		bootFile = fmt.Sprintf("%s/boot.ipxe?mac=${net0/mac}", serverURL)
	} else if httpBoot {
		// HTTP boot — serve iPXE binary as full URL
		switch arch {
		case iana.EFI_ARM64:
			bootFile = fmt.Sprintf("%s/ipxe-arm64.efi", serverURL)
		default:
			bootFile = fmt.Sprintf("%s/ipxe.efi", serverURL)
		}
	} else {
		// Raw PXE - serve the right iPXE binary via TFTP
		switch arch {
		case iana.EFI_X86_64, iana.EFI_BC:
			bootFile = "ipxe.efi"
		case iana.EFI_ARM64:
			bootFile = "ipxe-arm64.efi"
		default:
			// BIOS / IA32 / unknown → legacy
			bootFile = "undionly.kpxe"
		}
	}

	opts := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer),
		dhcpv4.WithServerIP(s.ServerIP),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.ServerIP)),
		dhcpv4.WithOption(dhcpv4.OptBootFileName(bootFile)),
	}
	if httpBoot {
		opts = append(opts, dhcpv4.WithOption(dhcpv4.OptClassIdentifier("HTTPClient")))
	} else {
		opts = append(opts, dhcpv4.WithOption(dhcpv4.OptClassIdentifier("PXEClient")))
		opts = append(opts, dhcpv4.WithOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, vendorOpts())))
	}

	resp, err := dhcpv4.NewReplyFromRequest(pkt, opts...)
	if err != nil {
		log.Printf("proxydhcp: reply error: %v", err)
		return
	}

	// For REQUESTs, respond with ACK
	if pkt.MessageType() == dhcpv4.MessageTypeRequest {
		resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	}

	// Set next-server (siaddr) for TFTP — only needed for PXE, not HTTP boot
	if !httpBoot {
		resp.ServerIPAddr = s.ServerIP
	}

	// Don't assign an IP - this is proxy DHCP
	resp.YourIPAddr = net.IPv4(0, 0, 0, 0)

	if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
		log.Printf("proxydhcp: send error: %v", err)
	}

	log.Printf("proxydhcp: → %s boot=%s method=%s", pkt.ClientHWAddr, bootFile, method)
}

func isPXEClient(pkt *dhcpv4.DHCPv4) bool {
	vc := pkt.Options.Get(dhcpv4.OptionClassIdentifier)
	if vc == nil {
		return false
	}
	s := string(vc)
	return len(s) >= 9 && s[:9] == "PXEClient"
}

func isHTTPBootClient(pkt *dhcpv4.DHCPv4) bool {
	vc := pkt.Options.Get(dhcpv4.OptionClassIdentifier)
	if vc == nil {
		return false
	}
	s := string(vc)
	return len(s) >= 10 && s[:10] == "HTTPClient"
}

func isIPXEClient(pkt *dhcpv4.DHCPv4) bool {
	uc := pkt.Options.Get(dhcpv4.OptionUserClassInformation)
	if uc == nil {
		return false
	}
	return string(uc) == "iPXE" || string(uc) == "\x04iPXE"
}

func clientArch(pkt *dhcpv4.DHCPv4) iana.Arch {
	archs := pkt.ClientArch()
	if len(archs) == 0 {
		return iana.INTEL_X86PC // default to BIOS
	}
	return archs[0]
}

func archName(a iana.Arch) string {
	switch a {
	case iana.INTEL_X86PC:
		return "bios"
	case iana.EFI_X86_64:
		return "efi-x64"
	case iana.EFI_BC:
		return "efi-bc"
	case iana.EFI_ARM64:
		return "efi-arm64"
	case iana.EFI_IA32:
		return "efi-ia32"
	default:
		return fmt.Sprintf("unknown(%d)", a)
	}
}

// vendorOpts returns PXE vendor options telling the client we're a proxy
func vendorOpts() []byte {
	// PXE discovery control: disable multicast/broadcast discovery,
	// just use the boot server we provide
	return []byte{
		6, 1, 8, // Option 6 (PXE discovery control): value 8 = skip discovery
		255, // End
	}
}
