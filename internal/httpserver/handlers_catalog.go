package httpserver

import (
	"log"
	"net/http"

	"github.com/justinpopa/duh/internal/catalog"
	"github.com/justinpopa/duh/internal/db"
)

func (s *Server) handleCatalogPull(w http.ResponseWriter, r *http.Request) {
	catalogID := r.FormValue("catalog_id")
	if catalogID == "" {
		http.Error(w, "catalog_id required", http.StatusBadRequest)
		return
	}

	cat, err := catalog.Fetch(s.CatalogURL)
	if err != nil {
		http.Error(w, "Failed to fetch catalog", http.StatusInternalServerError)
		return
	}

	var entry *catalog.Entry
	for i := range cat.Entries {
		if cat.Entries[i].ID == catalogID {
			entry = &cat.Entries[i]
			break
		}
	}
	if entry == nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	force := r.FormValue("force") == "true"
	imageID, err := catalog.Pull(s.DB, s.DataDir, *entry, force)

	// Auto-create profile if the entry has config template / kernel params
	if err == nil || (err != nil && err.Error() == "already pulled") {
		pd := catalog.ProfileDataFromEntry(*entry)
		if pd != nil {
			existing, lookupErr := db.GetProfileByCatalogID(s.DB, entry.ID)
			if lookupErr == nil && existing == nil {
				_, createErr := db.CreateProfile(s.DB, pd.Name, pd.Description, pd.OSFamily,
					pd.ConfigTemplate, pd.KernelParams, pd.DefaultVars, "", pd.VarSchema, entry.ID)
				if createErr != nil {
					log.Printf("http: auto-create profile for %s: %v", entry.ID, createErr)
				} else {
					log.Printf("http: auto-created profile for catalog entry %s", entry.ID)
				}
			}
		}
	}

	if err != nil {
		if err.Error() == "already pulled" || err.Error() == "already downloading" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		log.Printf("http: catalog pull: %v", err)
		http.Error(w, "Failed to pull image", http.StatusInternalServerError)
		return
	}

	s.renderImageRow(w, imageID)
}
