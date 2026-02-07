package httpserver

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/ipxe"
	"github.com/justinpopa/duh/internal/profile"
	"github.com/justinpopa/duh/internal/tftpserver"
)

func (s *Server) handleBootScript(w http.ResponseWriter, r *http.Request) {
	mac := r.URL.Query().Get("mac")
	if mac == "" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(ipxe.ExitScript()))
		return
	}

	clientIP := clientAddr(r)

	// Auto-register: creates if unknown, touches last_seen if known
	sys, isNew, err := db.AutoRegister(s.DB, mac, clientIP)
	if err != nil {
		log.Printf("http: boot auto-register: %v", err)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(ipxe.ExitScript()))
		return
	}

	if isNew && sys != nil {
		s.fireSystemEvent(sys, "discovered")
	}

	if sys == nil || sys.State != "queued" || sys.ImageID == nil || sys.Hostname == "" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(ipxe.ExitScript()))
		return
	}

	img, err := db.GetImage(s.DB, *sys.ImageID)
	if err != nil || img == nil {
		log.Printf("http: boot image lookup: %v", err)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(ipxe.ExitScript()))
		return
	}

	serverURL := s.ServerURL
	if serverURL == "" {
		serverURL = "http://" + r.Host
	}

	// Helper to build and sign an image file URL
	imageFileURL := func(filename string) string {
		return s.signURL(fmt.Sprintf("%s/images/%d/file/%s", serverURL, img.ID, filename))
	}

	// Build kernel URL and extra file URLs based on boot type
	var kernelURL, initrdURL string
	var extraFileURLs ipxe.ExtraFileURLs

	switch img.BootType {
	case "wimboot":
		kernelURL = imageFileURL("wimboot")
		extraFileURLs.BCD = imageFileURL("BCD")
		extraFileURLs.BootSDI = imageFileURL("boot.sdi")
		extraFileURLs.BootWIM = imageFileURL("boot.wim")
	case "esxi":
		kernelURL = imageFileURL("mboot.efi")
		extraFileURLs.BootCfg = imageFileURL("boot.cfg")
	case "iso":
		kernelURL = imageFileURL("memdisk")
		extraFileURLs.BootISO = imageFileURL("boot.iso")
	default: // linux
		kernelURL = imageFileURL("vmlinuz")
		initrdURL = imageFileURL("initrd.img")
	}

	cmdline := img.Cmdline
	var prof *db.Profile

	// If system has a profile, render kernel_params and append to cmdline
	if sys.ProfileID != nil {
		var err error
		prof, err = db.GetProfile(s.DB, *sys.ProfileID)
		if err != nil {
			log.Printf("http: boot profile lookup: %v", err)
			// Graceful degradation: continue with original cmdline
		} else if prof != nil && prof.KernelParams != "" {
			vars, err := profile.BuildVars(prof.DefaultVars, sys.Vars)
			if err != nil {
				log.Printf("http: boot build vars: %v", err)
			} else {
				configURL := s.signURL(fmt.Sprintf("%s/config/%d", serverURL, sys.ID))
				callbackURL := s.signURL(fmt.Sprintf("%s/api/v1/systems/%s/callback", serverURL, sys.MAC))
				tv := profile.TemplateVars{
					MAC:         sys.MAC,
					Hostname:    sys.Hostname,
					IP:          sys.IPAddr,
					SystemID:    sys.ID,
					ImageID:     *sys.ImageID,
					ServerURL:   serverURL,
					ConfigURL:   configURL,
					CallbackURL: callbackURL,
					Vars:        vars,
				}
				rendered, err := profile.RenderKernelParams(prof.KernelParams, tv)
				if err != nil {
					log.Printf("http: boot render kernel_params: %v", err)
				} else if rendered != "" {
					cmdline = strings.TrimSpace(cmdline + " " + rendered)
				}
			}
		}
	}

	var overlayURLs []string
	if prof != nil && prof.OverlayFile != "" {
		overlayURLs = append(overlayURLs, s.signURL(fmt.Sprintf("%s/profiles/%d/overlay/%s", serverURL, prof.ID, prof.OverlayFile)))
	}

	params := ipxe.ScriptParams{
		KernelURL:     kernelURL,
		InitrdURL:     initrdURL,
		Cmdline:       cmdline,
		MAC:           sys.MAC,
		Hostname:      sys.Hostname,
		OverlayURLs:   overlayURLs,
		ExtraFileURLs: extraFileURLs,
	}

	script, err := ipxe.RenderBootScript(img.BootType, params, img.IPXEScript)
	if err != nil {
		log.Printf("http: render boot script: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	globalConfirm, _ := db.GetSetting(s.DB, "confirm_reimage")
	if globalConfirm == "1" {
		script = ipxe.WrapWithConfirmation(script, sys.Hostname, sys.MAC)
	}

	// Transition to provisioning state
	if err := db.UpdateSystemState(s.DB, sys.ID, "provisioning"); err != nil {
		log.Printf("http: boot state transition: %v", err)
	} else {
		s.fireSystemEvent(sys, "provisioning")
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(script))
}

func (s *Server) handleServeIPXE(w http.ResponseWriter, r *http.Request) {
	serveIPXEBinary(w, "ipxe.efi", "application/efi")
}

func (s *Server) handleServeIPXEArm64(w http.ResponseWriter, r *http.Request) {
	serveIPXEBinary(w, "ipxe-arm64.efi", "application/efi")
}

func (s *Server) handleServeUndionly(w http.ResponseWriter, r *http.Request) {
	serveIPXEBinary(w, "undionly.kpxe", "application/octet-stream")
}

func serveIPXEBinary(w http.ResponseWriter, name, contentType string) {
	data, err := tftpserver.GetIPXEBinary(name)
	if err != nil {
		http.Error(w, "iPXE binary not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Write(data)
}

func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
