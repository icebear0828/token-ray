package plugin

import (
	"path/filepath"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: 1,
		ID:            "demo",
		Name:          "Demo CLI",
		Source:        "Demo",
		Version:       "1.0.0",
		WatchDirs:     []string{"~/.demo/logs"},
		Fields: FieldMap{
			Model:        "model",
			InputTokens:  "usage.input",
			OutputTokens: "usage.output",
		},
	}
}

func TestManifestValidate(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"schema", func(m *Manifest) { m.SchemaVersion = 2 }},
		{"id", func(m *Manifest) { m.ID = "Bad ID" }},
		{"name", func(m *Manifest) { m.Name = " " }},
		{"source", func(m *Manifest) { m.Source = "" }},
		{"version", func(m *Manifest) { m.Version = "" }},
		{"watch dirs", func(m *Manifest) { m.WatchDirs = nil }},
		{"model field", func(m *Manifest) { m.Fields.Model = "" }},
		{"input field", func(m *Manifest) { m.Fields.InputTokens = "" }},
		{"input mode", func(m *Manifest) { m.InputMode = "weird" }},
		{"dedup", func(m *Manifest) { m.Dedup = "weird" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifest()
			tc.mutate(&m)
			if err := m.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestExpandWatchDirs(t *testing.T) {
	m := validManifest()
	dirs, err := m.ExpandWatchDirs()
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(dirs) != 1 || !filepath.IsAbs(dirs[0]) {
		t.Fatalf("unexpected dirs: %v", dirs)
	}

	m.WatchDirs = []string{filepath.Join(string(filepath.Separator), "definitely-outside-home")}
	if _, err := m.ExpandWatchDirs(); err == nil {
		t.Fatal("expected outside-home watch dir to be rejected")
	}
}

func TestMatcherDefaultsToJSONL(t *testing.T) {
	m := validManifest()
	match := m.Matcher()
	if !match("/logs/session.jsonl") {
		t.Fatal("expected .jsonl to match")
	}
	if match("/logs/session.log") {
		t.Fatal("did not expect .log to match")
	}
}

func TestVersionAtLeast(t *testing.T) {
	if !versionAtLeast("1.2.3", "1.2.3") {
		t.Fatal("equal versions should pass")
	}
	if !versionAtLeast("1.3.0", "1.2.9") {
		t.Fatal("newer version should pass")
	}
	if versionAtLeast("1.2.3", "1.3.0") {
		t.Fatal("older version should fail")
	}
	if !versionAtLeast("dev", "9.9.9") {
		t.Fatal("dev builds should skip the version check")
	}
}

func TestDecodeManifestStripsBOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(demoManifestJSON)...)
	if _, err := decodeManifest(data); err != nil {
		t.Fatalf("manifest with UTF-8 BOM should decode: %v", err)
	}
}
