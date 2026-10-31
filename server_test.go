package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeSpooler struct {
	printers []Printer
	jobs     []PrintJob
	fail     error
}

func (f *fakeSpooler) ListPrinters() ([]Printer, error) { return f.printers, nil }
func (f *fakeSpooler) Print(job PrintJob) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	f.jobs = append(f.jobs, job)
	return "job-1", nil
}

func newTestServer(t *testing.T) (*Server, *fakeSpooler, *httptest.Server) {
	t.Helper()
	sp := &fakeSpooler{printers: []Printer{{Name: "Zebra_ZD421", Default: true, State: "ready"}}}
	srv := &Server{cfg: &Config{Port: 1, Origins: defaultOrigins}, pairing: NewPairing(), spooler: sp, version: "test",
		scanners: func() ([]Scanner, error) {
			return []Scanner{{Name: "Voyager-1200g", Vendor: "Honeywell", ID: "0c2e:0b61"}}, nil
		}}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, sp, ts
}

const portal = "https://portal.uscompoundingfactory.com"

func do(t *testing.T, ts *httptest.Server, method, path, origin, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, ts.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	return res, out
}

func TestStatusListsPrintersAndPairingState(t *testing.T) {
	srv, _, ts := newTestServer(t)
	res, out := do(t, ts, "GET", "/v1/status", portal, "", nil)
	if res.StatusCode != 200 || out["paired"] != false {
		t.Fatalf("status %d %v", res.StatusCode, out)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != portal {
		t.Fatal("portal origin must be allowed")
	}
	printers := out["printers"].([]any)
	if len(printers) != 1 || printers[0].(map[string]any)["name"] != "Zebra_ZD421" {
		t.Fatalf("printers %v", printers)
	}
	scanners := out["scanners"].([]any)
	if len(scanners) != 1 || scanners[0].(map[string]any)["vendor"] != "Honeywell" {
		t.Fatalf("scanners %v", scanners)
	}
	srv.cfg.Token = "x"
	_, out = do(t, ts, "GET", "/v1/status", portal, "", nil)
	if out["paired"] != true {
		t.Fatal("paired should be reported")
	}
}

func TestOtherOriginsAreRefused(t *testing.T) {
	_, _, ts := newTestServer(t)
	res, _ := do(t, ts, "GET", "/v1/status", "https://evil.example", "", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.StatusCode)
	}
	// Preflight from the portal passes with the private-network header.
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/v1/print", nil)
	req.Header.Set("Origin", portal)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusNoContent || res.Header.Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatalf("preflight %d", res.StatusCode)
	}
}

func TestPairingCodeIsSingleUseAndPrintNeedsTheToken(t *testing.T) {
	srv, sp, ts := newTestServer(t)
	label := base64.StdEncoding.EncodeToString([]byte("^XA^FDtest^FS^XZ"))

	// Unpaired: printing is refused.
	res, _ := do(t, ts, "POST", "/v1/print", portal, "", map[string]any{"printer": "Zebra_ZD421", "format": "ZPL", "data": label})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unpaired print should be 401, got %d", res.StatusCode)
	}

	code := srv.pairing.NewCode()
	res, _ = do(t, ts, "POST", "/v1/pair", portal, "", map[string]any{"code": "000000"})
	if res.StatusCode != http.StatusUnauthorized && code != "000000" {
		t.Fatalf("wrong code should be 401, got %d", res.StatusCode)
	}
	res, out := do(t, ts, "POST", "/v1/pair", portal, "", map[string]any{"code": code})
	if res.StatusCode != 200 || out["token"] == nil {
		t.Fatalf("pair failed: %d %v", res.StatusCode, out)
	}
	token := out["token"].(string)
	res, _ = do(t, ts, "POST", "/v1/pair", portal, "", map[string]any{"code": code})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatal("a code must work only once")
	}

	res, out = do(t, ts, "POST", "/v1/print", portal, token, map[string]any{"printer": "Zebra_ZD421", "format": "zpl", "data": label})
	if res.StatusCode != 200 || out["ok"] != true || len(sp.jobs) != 1 || sp.jobs[0].Format != "ZPL" {
		t.Fatalf("print failed: %d %v jobs=%d", res.StatusCode, out, len(sp.jobs))
	}
	res, _ = do(t, ts, "POST", "/v1/print", portal, "wrong", map[string]any{"printer": "Zebra_ZD421", "format": "ZPL", "data": label})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatal("a wrong token must be refused")
	}
	res, out = do(t, ts, "POST", "/v1/print", portal, token, map[string]any{"printer": "Zebra_ZD421", "format": "DOCX", "data": label})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad format should be 400, got %d %v", res.StatusCode, out)
	}
	sp.fail = errPrinterOffline
	res, out = do(t, ts, "POST", "/v1/print", portal, token, map[string]any{"printer": "Zebra_ZD421", "format": "ZPL", "data": label})
	if res.StatusCode != http.StatusBadGateway || out["error"] != errPrinterOffline.Error() {
		t.Fatalf("offline printer: %d %v", res.StatusCode, out)
	}
}

func TestPairingCodeExpires(t *testing.T) {
	p := NewPairing()
	now := time.Now()
	p.now = func() time.Time { return now }
	code := p.NewCode()
	now = now.Add(pairingCodeTTL + time.Second)
	if _, ok := p.Redeem(code); ok {
		t.Fatal("an expired code must not pair")
	}
}
