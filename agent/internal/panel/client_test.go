package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func server(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()

	s := httptest.NewServer(h)
	t.Cleanup(s.Close)

	return New(s.URL+"/", "ptlc_secret")
}

func TestStateSendsTheKeyAndReadsTheState(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/client/servers/60455931/resources" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer ptlc_secret" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("bad headers: %v", r.Header)
		}

		io.WriteString(w, `{"object":"stats","attributes":{"current_state":"offline","is_suspended":false,"resources":{}}}`)
	})

	got, err := c.State(context.Background(), "60455931")
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
