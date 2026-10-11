package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()

	s := httptest.NewServer(h)
	t.Cleanup(s.Close)

	return New(s.URL+"/", "ptlc_secret")
}

func TestStateSendsTheKeyAndReadsTheState(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/client/servers/1a2b3c4d/resources" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer ptlc_secret" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("bad headers: %v", r.Header)
		}

		io.WriteString(w, `{"object":"stats","attributes":{"current_state":"offline","is_suspended":false,"resources":{}}}`)
	})

	got, err := c.State(context.Background(), "1a2b3c4d")
	if err != nil || got != "offline" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestStateReportsSuspended(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"attributes":{"current_state":"offline","is_suspended":true}}`)
	})

	if got, _ := c.State(context.Background(), "x"); got != "suspended" {
		t.Fatalf("got %q", got)
	}
}

func TestStateAcceptsAFlatResponse(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"current_state":"running"}`) })

	if got, err := c.State(context.Background(), "x"); err != nil || got != "running" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestStateErrors(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"http error":     func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", 403) },
		"empty state":    func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"attributes":{}}`) },
		"not json":       func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `<html>`) },
		"server problem": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
	}

	for name, h := range cases {
		if _, err := server(t, h).State(context.Background(), "x"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestErrorsNeverContainTheKey(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", 401) })

	_, err := c.State(context.Background(), "x")
	if err == nil || strings.Contains(err.Error(), "ptlc_secret") {
		t.Fatalf("error %v", err)
	}
}

func TestStartPostsTheSignal(t *testing.T) {
	var body map[string]string

	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/client/servers/abc/power" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&body)

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing content type")
		}

		w.WriteHeader(204)
	})

	if err := c.Start(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}

	if body["signal"] != "start" || len(body) != 1 {
		t.Fatalf("body %v", body)
	}
}

func TestStartFailure(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "forbidden", 403) })

	if err := c.Start(context.Background(), "abc"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("got %v", err)
	}
}

func TestServerIDIsEscaped(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.EscapedPath(), "../") {
			t.Errorf("path traversal not escaped: %s", r.URL.EscapedPath())
		}

		io.WriteString(w, `{"attributes":{"current_state":"offline"}}`)
	})

	c.State(context.Background(), "../../account")
}

func TestDetails(t *testing.T) {
	cases := map[string]struct {
		body string
		want bool
	}{
		"has start":  {`{"attributes":{"name":"Valheim"},"meta":{"is_server_owner":false,"user_permissions":["control.start","websocket.connect"]}}`, true},
		"no start":   {`{"attributes":{"name":"Valheim"},"meta":{"user_permissions":["websocket.connect"]}}`, false},
		"wildcard":   {`{"attributes":{"name":"Valheim"},"meta":{"is_server_owner":true,"user_permissions":["*"]}}`, true},
		"no meta":    {`{"attributes":{"name":"Valheim"}}`, false},
		"empty list": {`{"attributes":{"name":"Valheim"},"meta":{"user_permissions":[]}}`, false},
	}

	for name, c := range cases {
		cl := server(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, c.body) })

		d, err := cl.Details(context.Background(), "x")
		if err != nil || d.CanStart != c.want || d.Name != "Valheim" {
			t.Errorf("%s: %+v %v", name, d, err)
		}
	}
}

func TestPing(t *testing.T) {
	if err := server(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }).Ping(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := server(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 401) }).Ping(context.Background()); err == nil {
		t.Fatal("expected an error for a rejected key")
	}
}

func TestConfigFetchesAndParses(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/client/lazy-pelican/agent/config" || r.URL.Query().Get("node") != "my node" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}

		io.WriteString(w, `{"api_version":1,"detector":{"packets":3,"window_seconds":5,"holdoff_seconds":30},"limits":{"cooldown_seconds":300,"max_wakes_per_hour":6},
			"servers":[{"identifier":"5e6f7a8b","name":"Valheim","node":"node","ports":[27000],"match":{"prefix":"2010000d","length":512}}]}`)
	})

	cfg, err := c.Config(context.Background(), "my node")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Detector.Packets != 3 || cfg.Limits.MaxWakesPerHour != 6 || len(cfg.Servers) != 1 || cfg.Servers[0].Ports[0] != 27000 || cfg.Servers[0].Match.Length != 512 {
		t.Fatalf("%+v", cfg)
	}
}

func TestConfigWithoutANodeSendsNoQuery(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}

		io.WriteString(w, `{"api_version":1,"servers":[]}`)
	})

	if _, err := c.Config(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestConfigAndHeartbeat404MeansTheRouteIsMissing(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	if _, err := c.Config(context.Background(), ""); !errors.Is(err, ErrNotSupported) {
		t.Errorf("config: %v", err)
	}

	if err := c.SendHeartbeat(context.Background(), Heartbeat{}); !errors.Is(err, ErrNotSupported) {
		t.Errorf("heartbeat: %v", err)
	}
}

func TestConfigOtherErrorsAreNotMistakenForAMissingPlugin(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", 403) })

	_, err := c.Config(context.Background(), "")
	if err == nil || errors.Is(err, ErrNotSupported) {
		t.Fatalf("got %v", err)
	}
}

func TestSendHeartbeatBody(t *testing.T) {
	var got map[string]any

	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/client/lazy-pelican/agent/heartbeat" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(204)
	})

	msg := "operation not permitted"
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	err := c.SendHeartbeat(context.Background(), Heartbeat{
		AgentVersion: "1.2.3", Hostname: "gameserver", Node: "node", Interface: "any",
		Capture: CaptureReport{OK: false, Error: &msg},
		Servers: []ServerReport{{Identifier: "abc123", LastDetectionAt: &at}},
	})
	if err != nil {
		t.Fatal(err)
	}

	capture := got["capture"].(map[string]any)
	servers := got["servers"].([]any)
	first := servers[0].(map[string]any)

	if got["agent_version"] != "1.2.3" || got["hostname"] != "gameserver" || got["node"] != "node" || capture["ok"] != false || capture["error"] != msg ||
		first["identifier"] != "abc123" || first["last_detection_at"] != "2026-01-01T12:00:00Z" || first["last_wake_at"] != nil {
		t.Fatalf("unexpected body: %v", got)
	}
}

func TestSendHeartbeatWithNoServersSendsAnEmptyList(t *testing.T) {
	var raw string

	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		raw = string(b)
		w.WriteHeader(204)
	})

	if err := c.SendHeartbeat(context.Background(), Heartbeat{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(raw, `"servers":[]`) {
		t.Fatalf("the panel validates servers as an array, so null is wrong: %s", raw)
	}
}

func TestReportWake(t *testing.T) {
	var got map[string]string

	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/client/lazy-pelican/agent/wake" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(204)
	})

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.FixedZone("x", 3600))

	if err := c.ReportWake(context.Background(), "abc123", at); err != nil {
		t.Fatal(err)
	}

	if got["identifier"] != "abc123" || got["at"] != "2026-01-01T11:00:00Z" {
		t.Fatalf("%v", got)
	}
}

func TestReportWake404IsNotAMissingPlugin(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "No such server", 404) })

	err := c.ReportWake(context.Background(), "abc123", time.Now())
	if err == nil || errors.Is(err, ErrNotSupported) {
		t.Fatalf("got %v", err)
	}
}
