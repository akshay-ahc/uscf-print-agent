package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Server is the local HTTP API the station page calls on 127.0.0.1:
//
//	GET  /v1/status  -> version, paired, printers           (no auth)
//	POST /v1/pair    -> { code } -> { token }               (single-use code from the tray)
//	POST /v1/print   -> { printer, format, data(base64) }   (Authorization: Bearer <token>)
//
// Only the configured portal origins pass CORS, so another website open in the
// same browser cannot reach the agent.
type Server struct {
	cfg     *Config
	pairing *Pairing
	spooler Spooler
	version string
	// onPaired lets the tray refresh its menu after a successful pairing.
	onPaired func()
}

const maxLabelBytes = 8 << 20 // a 4x6 PDF is well under 1 MB; ZPL a few KB

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/pair", s.handlePair)
	mux.HandleFunc("/v1/print", s.handlePrint)
	return s.cors(mux)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !s.cfg.AllowsOrigin(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			// Chrome's Private Network Access check for https pages calling localhost.
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusResponse struct {
	Version  string    `json:"version"`
	Paired   bool      `json:"paired"`
	Printers []Printer `json:"printers"`
	Error    string    `json:"error,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := statusResponse{Version: s.version, Paired: s.cfg.Token != ""}
	printers, err := s.spooler.ListPrinters()
	if err != nil {
		resp.Error = "could not list printers: " + err.Error()
	}
	if printers == nil {
		printers = []Printer{}
	}
	resp.Printers = printers
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "send the 6-digit code"})
		return
	}
	token, ok := s.pairing.Redeem(strings.TrimSpace(body.Code))
	if !ok {
		// A wrong guess costs a second, so a stray script cannot run through the codes.
		time.Sleep(time.Second)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "that code is wrong or has expired; get a new one from the printer icon"})
		return
	}
	s.cfg.Token = token
	if err := s.cfg.Save(); err != nil {
		log.Printf("save config: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "could not save the pairing on this computer"})
		return
	}
	if s.onPaired != nil {
		s.onPaired()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": token})
}

func (s *Server) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	return tokenMatches(s.cfg.Token, strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "this computer is not paired with the station; pair it from the printer icon"})
		return
	}
	var body struct {
		Printer string `json:"printer"`
		Format  string `json:"format"`
		Data    string `json:"data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLabelBytes*4/3+4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad print request"})
		return
	}
	body.Format = strings.ToUpper(strings.TrimSpace(body.Format))
	if !validFormat(body.Format) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": errBadFormat.Error()})
		return
	}
	if strings.TrimSpace(body.Printer) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "choose a printer first"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil || len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "the label data is empty or not base64"})
		return
	}
	if len(data) > maxLabelBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"ok": false, "error": "the label is too large to print"})
		return
	}
	jobID, err := s.spooler.Print(PrintJob{Printer: body.Printer, Format: body.Format, Data: data})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errNoPrinter) || errors.Is(err, errBadFormat) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("printed %s label (%d bytes) on %q, job %s", body.Format, len(data), body.Printer, jobID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job_id": jobID})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func (s *Server) addr() string { return fmt.Sprintf("127.0.0.1:%d", s.cfg.Port) }
