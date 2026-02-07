//go:build !linux

package httpserver

import "net/http"

func (s *Server) handleDHCPTest(w http.ResponseWriter, r *http.Request) {
	renderDHCPError(w, s, "DHCP test is only available on Linux.\n\nThe DHCP client requires raw sockets which are only supported on Linux.")
}
