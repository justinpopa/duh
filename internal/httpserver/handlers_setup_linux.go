package httpserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/justinpopa/duh/internal/proxydhcp"
)

func (s *Server) handleDHCPTest(w http.ResponseWriter, r *http.Request) {
	ifaceName, _, err := proxydhcp.DetectInterface()
	if err != nil {
		renderDHCPError(w, s, fmt.Sprintf("Failed to detect network interface: %v", err))
		return
	}

	client, err := nclient4.New(ifaceName,
		nclient4.WithTimeout(5*time.Second),
		nclient4.WithRetry(1),
	)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "permission denied") ||
			strings.Contains(errMsg, "operation not permitted") ||
			strings.Contains(errMsg, "EPERM") {
			renderDHCPError(w, s, "Permission denied: raw sockets require root privileges.\n\nRun with sudo:\n  sudo duh\n\nOr use:\n  make dev-pxe")
		} else {
			renderDHCPError(w, s, fmt.Sprintf("Failed to create DHCP client: %v", err))
		}
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	offer, err := client.DiscoverOffer(ctx)
	if err != nil {
		renderDHCPError(w, s, fmt.Sprintf("No DHCP response received: %v", err))
		return
	}

	// Parse key options from the offer
	var options []DHCPOption

	if v := offer.Options.Get(dhcpv4.OptionServerIdentifier); v != nil {
		options = append(options, DHCPOption{54, "Server Identifier", net.IP(v).String()})
	}

	if v := offer.SubnetMask(); v != nil {
		options = append(options, DHCPOption{1, "Subnet Mask", net.IP(v).String()})
	}

	if routers := offer.Router(); len(routers) > 0 {
		parts := make([]string, len(routers))
		for i, r := range routers {
			parts[i] = r.String()
		}
		options = append(options, DHCPOption{3, "Router", strings.Join(parts, ", ")})
	}

	if dns := offer.DNS(); len(dns) > 0 {
		parts := make([]string, len(dns))
		for i, d := range dns {
			parts[i] = d.String()
		}
		options = append(options, DHCPOption{6, "DNS Servers", strings.Join(parts, ", ")})
	}

	if v := offer.TFTPServerName(); v != "" {
		options = append(options, DHCPOption{66, "TFTP Server Name", v})
	}

	if v := offer.BootFileNameOption(); v != "" {
		options = append(options, DHCPOption{67, "Boot File Name (opt 67)", v})
	} else if offer.BootFileName != "" {
		options = append(options, DHCPOption{67, "Boot File Name (file)", offer.BootFileName})
	}

	if v := offer.Options.Get(dhcpv4.OptionClassIdentifier); v != nil {
		options = append(options, DHCPOption{60, "Vendor Class ID", string(v)})
	}

	if v := offer.Options.Get(dhcpv4.OptionVendorSpecificInformation); v != nil {
		options = append(options, DHCPOption{43, "Vendor Specific Info", fmt.Sprintf("%x", v)})
	}

	nextServer := offer.ServerIPAddr.String()
	hasBootOptions := offer.TFTPServerName() != "" ||
		offer.BootFileNameOption() != "" ||
		offer.BootFileName != "" ||
		!offer.ServerIPAddr.IsUnspecified()

	data := map[string]any{
		"Success":        true,
		"OfferedIP":      offer.YourIPAddr.String(),
		"NextServer":     nextServer,
		"Options":        options,
		"HasBootOptions": hasBootOptions,
	}

	if v := offer.Options.Get(dhcpv4.OptionServerIdentifier); v != nil {
		data["ServerIP"] = net.IP(v).String()
	} else {
		data["ServerIP"] = "unknown"
	}

	if err := s.Templates.ExecuteTemplate(w, "dhcp_test_result", data); err != nil {
		log.Printf("http: render dhcp test result: %v", err)
	}
}
