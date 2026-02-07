package httpserver

import (
	"log"
	"net/http"
)

// DHCPOption is a parsed DHCP option for template rendering.
type DHCPOption struct {
	Code  uint8
	Name  string
	Value string
}

func renderDHCPError(w http.ResponseWriter, s *Server, msg string) {
	data := map[string]any{
		"Success": false,
		"Error":   msg,
	}
	if err := s.Templates.ExecuteTemplate(w, "dhcp_test_result", data); err != nil {
		log.Printf("http: render dhcp test error: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}
