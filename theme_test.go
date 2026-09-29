package tint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadThemeConfigEmptyPath(t *testing.T) {
	_, err := LoadThemeConfig("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestLoadThemeConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Error("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestLoadThemeConfigInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadThemeConfigValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	data := []byte(`{"palette":{"primary":{"color":"#ff0000"}},"styles":{"foo":{"foreground":"primary"}},"huh":{"focused_title":{"foreground":"primary"}}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadThemeConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Palette["primary"].Raw != "#ff0000" {
		t.Errorf("primary color = %q, want #ff0000", cfg.Palette["primary"].Raw)
	}
	if cfg.Styles["foo"].Foreground != "primary" {
		t.Errorf("foo foreground = %q, want primary", cfg.Styles["foo"].Foreground)
	}
	if cfg.Huh["focused_title"].Foreground != "primary" {
		t.Errorf("focused_title foreground = %q, want primary", cfg.Huh["focused_title"].Foreground)
	}
}

func TestColorSpecUnmarshalString(t *testing.T) {
	var c ColorSpec
	if err := json.Unmarshal([]byte(`"#ff0000"`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Raw != "#ff0000" {
		t.Errorf("Raw = %q, want #ff0000", c.Raw)
	}
	if c.IsAdaptive() {
		t.Error("string form should not be adaptive")
	}
}

func TestColorSpecUnmarshalObject(t *testing.T) {
	var c ColorSpec
	if err := json.Unmarshal([]byte(`{"light":"#ff0000","dark":"#00ff00"}`), &c); err != nil {
		t.Fatal(err)
	}
	if !c.IsAdaptive() {
		t.Error("object form should be adaptive")
	}
	if c.Light != "#ff0000" || c.Dark != "#00ff00" {
		t.Errorf("Light/Dark = %q/%q, want #ff0000/#00ff00", c.Light, c.Dark)
	}
}

func TestParseLiteralColor(t *testing.T) {
	for _, tc := range []struct {
		input string
		empty bool
	}{
		{"#ff0000", false},
		{"red", false},
		{"212", false},
		{"none", true},
		{"", true},
	} {
		c := parseLiteralColor(tc.input)
		if tc.empty && c != nil {
			t.Errorf("parseLiteralColor(%q) = %v, want nil", tc.input, c)
		}
		if !tc.empty && c == nil {
			t.Errorf("parseLiteralColor(%q) = nil, want non-nil", tc.input)
		}
	}
}

func TestNewThemeDefaults(t *testing.T) {
	th := NewTheme(nil)
	if th == nil {
		t.Fatal("NewTheme(nil) returned nil")
	}
	if th.Style("highlight").GetForeground() == nil {
		t.Error("default highlight style has no foreground")
	}
}

func TestStyleUnknown(t *testing.T) {
	th := NewTheme(nil)
	s := th.Style("does-not-exist")
	if s.Render("plain") != "plain" {
		t.Error("unknown style should render plain text")
	}
}

func TestColorKnownAndLiteral(t *testing.T) {
	th := NewTheme(nil)
	if th.Color("primary") == nil {
		t.Error("primary color is nil")
	}
	if th.Color("#00ff00") == nil {
		t.Error("literal hex color is nil")
	}
}

func writeThemeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewThemeEmptyConfigEqualsDefaults(t *testing.T) {
	def := NewTheme(nil)
	empty := NewTheme(&ThemeConfig{})

	for name := range DefaultThemeConfig().Styles {
		if got, want := empty.Style(name).Render("sample"), def.Style(name).Render("sample"); got != want {
			t.Errorf("style %q: empty config = %q, want %q", name, got, want)
		}
	}
	for name := range DefaultThemeConfig().Palette {
		if got, want := empty.Color(name), def.Color(name); got != want {
			t.Errorf("color %q: empty config = %v, want %v", name, got, want)
		}
	}
}

func TestNewThemeSparseConfigKeepsDefaults(t *testing.T) {
	th := NewTheme(&ThemeConfig{
		Styles: map[string]StyleDef{
			"highlight": {Foreground: "primary"},
		},
	})

	if th.Style("primary").GetForeground() == nil {
		t.Error("default primary style lost its foreground")
	}
	if th.Style("error").GetForeground() == nil {
		t.Error("default error style lost its foreground")
	}
	if th.Color("error") == nil {
		t.Error("default error color missing")
	}
}

func TestNewThemePerNameReplace(t *testing.T) {
	th := NewTheme(&ThemeConfig{
		Styles: map[string]StyleDef{
			"highlight": {Foreground: "primary"},
		},
	})

	if th.Style("highlight").GetBold() {
		t.Error("user highlight should replace the default and not be bold")
	}
	if !NewTheme(nil).Style("highlight").GetBold() {
		t.Error("default highlight should be bold")
	}
}

func TestLoadThemeConfigUnknownTopLevelKey(t *testing.T) {
	path := writeThemeFile(t, `{"palette":{"primary":"#ff0000"},"extra":true}`)
	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown top-level key")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Errorf("error %q should name the offending key", err)
	}
}

func TestLoadThemeConfigUnknownStyleField(t *testing.T) {
	path := writeThemeFile(t, `{"styles":{"foo":{"foregound":"primary"}}}`)
	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown style field")
	}
	if !strings.Contains(err.Error(), "foregound") {
		t.Errorf("error %q should name the offending field", err)
	}
}

func TestLoadThemeConfigUnknownHuhKey(t *testing.T) {
	path := writeThemeFile(t, `{"huh":{"focussed_title":{"bold":true}}}`)
	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown huh key")
	}
	if !strings.Contains(err.Error(), "focussed_title") {
		t.Errorf("error %q should name the offending key", err)
	}
}

func TestLoadThemeConfigBadPaletteReference(t *testing.T) {
	path := writeThemeFile(t, `{"styles":{"foo":{"foreground":"primry"}}}`)
	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown palette reference")
	}
	if !strings.Contains(err.Error(), "primry") {
		t.Errorf("error %q should name the offending color", err)
	}
}

func TestLoadThemeConfigBadPaletteValue(t *testing.T) {
	path := writeThemeFile(t, `{"palette":{"primary":"notacolor"}}`)
	_, err := LoadThemeConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid palette value")
	}
	if !strings.Contains(err.Error(), "notacolor") {
		t.Errorf("error %q should name the offending color", err)
	}
}

func TestLoadThemeConfigValidFullOverride(t *testing.T) {
	path := writeThemeFile(t, `{"palette":{"primary":"#123456"},"styles":{"primary":{"foreground":"primary","italic":true}},"huh":{"focused_title":{"bold":true}}}`)
	cfg, err := LoadThemeConfig(path)
	if err != nil {
		t.Fatalf("load valid theme: %v", err)
	}
	th := NewTheme(cfg)
	if !th.Style("primary").GetItalic() {
		t.Error("user primary style should be italic")
	}
	if got := th.Color("primary"); got == nil {
		t.Error("primary color missing after override")
	}
}

func TestIsLiteralColor(t *testing.T) {
	valid := []string{"", "none", "#ff0000", "#fff", "#ffff", "#ff0000ff", "red", "bright_red", "212", "0", "255"}
	invalid := []string{"primry", "#gggggg", "#12345", "256", "-1", "notacolor"}
	for _, v := range valid {
		if !isLiteralColor(v) {
			t.Errorf("isLiteralColor(%q) = false, want true", v)
		}
	}
	for _, v := range invalid {
		if isLiteralColor(v) {
			t.Errorf("isLiteralColor(%q) = true, want false", v)
		}
	}
}

func TestDiffDefaults(t *testing.T) {
	cfg := &ThemeConfig{
		Palette: map[string]ColorSpec{
			"primary": DefaultThemeConfig().Palette["primary"],
		},
		Styles: map[string]StyleDef{
			"highlight": {Foreground: "primary"},
			"custom":    {Bold: true},
		},
	}

	diffs := cfg.DiffDefaults()
	byKey := make(map[string]KeyDiff, len(diffs))
	for _, d := range diffs {
		byKey[d.Section+"."+d.Key] = d
	}

	if got := byKey["palette.primary"].Status; got != DiffRedundant {
		t.Errorf("palette.primary = %q, want redundant", got)
	}
	if got := byKey["palette.error"].Status; got != DiffDefault {
		t.Errorf("palette.error = %q, want default", got)
	}

	hl := byKey["styles.highlight"]
	if hl.Status != DiffOverride {
		t.Errorf("styles.highlight = %q, want override", hl.Status)
	}
	if len(hl.DroppedFields) != 1 || hl.DroppedFields[0] != "bold" {
		t.Errorf("styles.highlight dropped fields = %v, want [bold]", hl.DroppedFields)
	}

	if got := byKey["styles.custom"].Status; got != DiffOverride {
		t.Errorf("styles.custom = %q, want override", got)
	}
	if got := byKey["styles.primary"].Status; got != DiffDefault {
		t.Errorf("styles.primary = %q, want default", got)
	}

	if !strings.Contains(hl.String(), "bold") {
		t.Errorf("KeyDiff.String() = %q, should mention dropped bold", hl.String())
	}
}

func TestDiffDefaultsNilIsAllDefault(t *testing.T) {
	var cfg *ThemeConfig
	for _, d := range cfg.DiffDefaults() {
		if d.Status != DiffDefault {
			t.Errorf("%s = %q, want default", d, d.Status)
		}
	}
}

func TestLipglossList(t *testing.T) {
	th := NewTheme(nil)
	out := LipglossList(th.Style("text"), []string{"one", "two"})
	if !strings.Contains(out, "one") {
		t.Error("list output missing 'one'")
	}
	if !strings.Contains(out, "two") {
		t.Error("list output missing 'two'")
	}
	if !strings.Contains(out, DefaultListBullet) {
		t.Error("list output missing bullet")
	}
}
