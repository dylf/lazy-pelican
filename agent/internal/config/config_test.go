package config

import (
	"strings"
	"testing"
	"time"
)

const minimal = `
panel_url: https://panel.example.com/
api_key: secret
servers:
  - name: palworld
    id: 1a2b3c4d
    ports: [27002]
`

func TestMinimalConfigGetsDefaults(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	c, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}

	if c.PanelURL != "https://panel.example.com" {
		t.Errorf("trailing slash not trimmed: %q", c.PanelURL)
	}

	// Detector and limit values stay zero ("not overridden") so the panel's values apply; see agent.Resolve.
	if c.Interface != "any" || c.Detector != (Detector{}) || c.Limits != (Limits{}) {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestServersAreOptionalBecauseThePanelSuppliesThem(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	c, err := Parse([]byte("panel_url: https://p\napi_key: k\nnode: node\n"))
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Servers) != 0 || c.Node != "node" {
		t.Fatalf("%+v", c)
	}
}

func TestDetectorAndLimitOverridesParse(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	c, err := Parse([]byte(minimal + "detector: {packets: 4, window: 8s, holdoff: 1m}\nlimits: {cooldown: 10m, max_wakes_per_hour: 2}\n"))
	if err != nil {
		t.Fatal(err)
	}

	if c.Detector.Packets != 4 || c.Detector.Window != 8*time.Second || c.Detector.Holdoff != time.Minute || c.Limits.Cooldown != 10*time.Minute || c.Limits.MaxWakesPerHour != 2 {
		t.Fatalf("%+v", c)
	}
}

func TestEnvOverridesTheKey(t *testing.T) {
	t.Setenv(APIKeyEnv, "from-env")

	c, err := Parse([]byte(strings.Replace(minimal, "api_key: secret", "api_key: from-file", 1)))
	if err != nil {
		t.Fatal(err)
	}

	if c.APIKey != "from-env" {
		t.Fatalf("got %q", c.APIKey)
	}
}

func TestKeyMayComeOnlyFromEnv(t *testing.T) {
	t.Setenv(APIKeyEnv, "from-env")

	if _, err := Parse([]byte(strings.Replace(minimal, "api_key: secret\n", "", 1))); err != nil {
		t.Fatal(err)
	}
}

func TestInvalid(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	cases := map[string]string{
		"no key":            strings.Replace(minimal, "api_key: secret\n", "", 1),
		"bad url":           strings.Replace(minimal, "https://panel.example.com/", "panel.example.com", 1),
		"typo in key":       minimal + "\nintrface: eth0\n",
		"no ports":          strings.Replace(minimal, "ports: [27002]", "ports: []", 1),
		"port 0":            strings.Replace(minimal, "[27002]", "[0]", 1),
		"no id":             strings.Replace(minimal, "id: 1a2b3c4d\n", "", 1),
		"bad prefix":        strings.Replace(minimal, "ports: [27002]", "ports: [27002]\n    match: {prefix: zz}", 1),
		"negative length":   strings.Replace(minimal, "ports: [27002]", "ports: [27002]\n    match: {length: -1}", 1),
		"bad ignore":        minimal + "\nignore_sources: [not-an-ip]\n",
		"bad packets":       minimal + "\ndetector: {packets: -1}\n",
		"negative window":   minimal + "\ndetector: {window: -5s}\n",
		"bad cap":           minimal + "\nlimits: {max_wakes_per_hour: -1}\n",
		"negative cooldown": minimal + "\nlimits: {cooldown: -1m}\n",
		"duplicate port":    minimal + "  - name: other\n    id: x\n    ports: [27002]\n",
		"duplicate name":    minimal + "  - name: palworld\n    id: x\n    ports: [27003]\n",
		"not even yaml":     "{{{",
		"unparsable window": minimal + "\ndetector: {window: soon}\n",
	}

	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestIgnorePrefixes(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	c, err := Parse([]byte(minimal + "\nignore_sources: [10.0.0.50, 192.168.0.0/16, '::1']\n"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.IgnorePrefixes()
	if err != nil || len(got) != 3 || got[0].String() != "10.0.0.50/32" || got[1].String() != "192.168.0.0/16" || got[2].String() != "::1/128" {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestRedactedDescribesWhereServersComeFrom(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	c, _ := Parse([]byte("panel_url: https://p\napi_key: k\n"))
	if !strings.Contains(c.Redacted(), "from the panel") {
		t.Errorf("%s", c.Redacted())
	}

	c, _ = Parse([]byte(minimal))
	if !strings.Contains(c.Redacted(), "1 from this file") {
		t.Errorf("%s", c.Redacted())
	}
}

func TestRedactedNeverContainsTheKey(t *testing.T) {
	t.Setenv(APIKeyEnv, "")

	c, _ := Parse([]byte(minimal))
	if strings.Contains(c.Redacted(), "secret") {
		t.Fatal("the API key leaked into Redacted()")
	}
}
