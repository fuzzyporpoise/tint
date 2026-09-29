// Package tint provides an opinionated theme loader and compiler for Go
// CLI/TUI projects built on charmbracelet's lipgloss and huh.
package tint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// -----------------------------------------------------------------------------
// Configuration Types
// -----------------------------------------------------------------------------

// ColorSpec represents a colour value. In JSON it can be:
//   - A plain string: hex ("#FF5FAF"), ANSI 256 index ("212"), or ANSI name.
//   - An object with "light" and "dark" keys for adaptive colours.
type ColorSpec struct {
	Raw   string `json:"color,omitempty"`
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
}

// UnmarshalJSON handles both string and object forms for a ColorSpec.
func (c *ColorSpec) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Raw = s
		return nil
	}
	type alt ColorSpec
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode((*alt)(c))
}

// IsAdaptive reports whether the ColorSpec has light/dark variants.
func (c ColorSpec) IsAdaptive() bool {
	return c.Light != "" || c.Dark != ""
}

// StyleDef defines a reusable lipgloss style. Only non-zero fields are emitted.
type StyleDef struct {
	Foreground       string `json:"foreground,omitempty"`
	Background       string `json:"background,omitempty"`
	BorderForeground string `json:"border_foreground,omitempty"`
	Bold             bool   `json:"bold,omitempty"`
	Italic           bool   `json:"italic,omitempty"`
	Underline        bool   `json:"underline,omitempty"`
	Strikethrough    bool   `json:"strikethrough,omitempty"`
	Faint            bool   `json:"faint,omitempty"`
	Blink            bool   `json:"blink,omitempty"`
	Reverse          bool   `json:"reverse,omitempty"`
}

// ThemeConfig is the raw JSON representation of a user theme.
type ThemeConfig struct {
	Palette map[string]ColorSpec `json:"palette"`
	Styles  map[string]StyleDef  `json:"styles"`
	Huh     map[string]StyleDef  `json:"huh"`
}

// -----------------------------------------------------------------------------
// Compiled Theme (immutable)
// -----------------------------------------------------------------------------

// Theme is a compiled, ready-to-use theme with resolved colors and styles.
type Theme struct {
	hasDarkBg      bool
	colors         map[string]color.Color
	styles         map[string]lipgloss.Style
	huhDefinitions map[string]StyleDef
}

// NewTheme compiles a ThemeConfig into a Theme with resolved colors and styles.
//
// The supplied config is always merged over DefaultThemeConfig(): unspecified
// palette entries and styles fall back to the built-in defaults, so both
// NewTheme(nil) and NewTheme(&ThemeConfig{}) produce the default theme. Merging
// is per name and replaces wholesale, not field by field: a user palette entry
// for "primary" replaces the default "primary", and a user StyleDef for a
// style or huh key replaces that entry entirely. Omit a name to keep its
// default.
func NewTheme(cfg *ThemeConfig) *Theme {
	merged := mergeThemeConfig(DefaultThemeConfig(), cfg)

	hasDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	t := &Theme{
		hasDarkBg:      hasDark,
		colors:         make(map[string]color.Color),
		styles:         make(map[string]lipgloss.Style),
		huhDefinitions: merged.Huh,
	}

	for name, spec := range merged.Palette {
		t.colors[name] = t.resolveColorSpec(spec)
	}
	for name, def := range merged.Styles {
		t.styles[name] = t.buildStyle(def)
	}

	return t
}

// Color looks up a named color from the palette, falling back to parsing the name as a literal color.
func (t *Theme) Color(name string) color.Color {
	if c, ok := t.colors[name]; ok {
		return c
	}
	return parseLiteralColor(name)
}

// Style looks up a named style, returning an empty style if the name is not found.
func (t *Theme) Style(name string) lipgloss.Style {
	if s, ok := t.styles[name]; ok {
		return s
	}
	return lipgloss.NewStyle()
}

// HuhTheme returns a huh.ThemeFunc that overlays custom styles onto a base theme.
func (t *Theme) HuhTheme(interactive bool) huh.ThemeFunc {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		base := huh.ThemeBase(isDark)
		if interactive {
			base = huh.ThemeCharm(isDark)
		}

		for _, ht := range huhTargets {
			def, ok := t.huhDefinitions[ht.key]
			if !ok {
				continue
			}
			target := ht.target(base)
			*target = t.overlayStyleDef(*target, def)
		}

		return base
	})
}

// huhTarget pairs a huh style key with an accessor that resolves the matching
// *lipgloss.Style on a freshly built *huh.Styles. It is the single source of
// truth for both applying overrides and validating huh keys.
type huhTarget struct {
	key    string
	target func(*huh.Styles) *lipgloss.Style
}

var huhTargets = []huhTarget{
	{"focused_title", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.Title }},
	{"focused_description", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.Description }},
	{"focused_selected_option", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.SelectedOption }},
	{"focused_unselected_option", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.UnselectedOption }},
	{"focused_error_indicator", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.ErrorIndicator }},
	{"focused_error_message", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.ErrorMessage }},
	{"focused_select_selector", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.SelectSelector }},
	{"focused_next_indicator", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.NextIndicator }},
	{"focused_prev_indicator", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.PrevIndicator }},
	{"focused_focused_button", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.FocusedButton }},
	{"focused_blurred_button", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.BlurredButton }},
	{"focused_directory", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.Directory }},
	{"focused_file", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.File }},
	{"focused_option", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.Option }},
	{"focused_multi_select_selector", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.MultiSelectSelector }},
	{"focused_selected_prefix", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.SelectedPrefix }},
	{"focused_unselected_prefix", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.UnselectedPrefix }},
	{"focused_card", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.Card }},
	{"focused_note_title", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.NoteTitle }},
	{"focused_next", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.Next }},

	{"blurred_title", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.Title }},
	{"blurred_description", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.Description }},
	{"blurred_selected_option", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.SelectedOption }},
	{"blurred_unselected_option", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.UnselectedOption }},
	{"blurred_error_indicator", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.ErrorIndicator }},
	{"blurred_error_message", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.ErrorMessage }},
	{"blurred_select_selector", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.SelectSelector }},
	{"blurred_next_indicator", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.NextIndicator }},
	{"blurred_prev_indicator", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.PrevIndicator }},
	{"blurred_focused_button", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.FocusedButton }},
	{"blurred_blurred_button", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.BlurredButton }},
	{"blurred_directory", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.Directory }},
	{"blurred_file", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.File }},
	{"blurred_option", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.Option }},
	{"blurred_multi_select_selector", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.MultiSelectSelector }},
	{"blurred_selected_prefix", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.SelectedPrefix }},
	{"blurred_unselected_prefix", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.UnselectedPrefix }},
	{"blurred_card", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.Card }},
	{"blurred_note_title", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.NoteTitle }},
	{"blurred_next", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.Next }},

	{"focused_textinput_cursor", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.TextInput.Cursor }},
	{"focused_textinput_cursor_text", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.TextInput.CursorText }},
	{"focused_textinput_placeholder", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.TextInput.Placeholder }},
	{"focused_textinput_prompt", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.TextInput.Prompt }},
	{"focused_textinput_text", func(s *huh.Styles) *lipgloss.Style { return &s.Focused.TextInput.Text }},

	{"blurred_textinput_cursor", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.TextInput.Cursor }},
	{"blurred_textinput_cursor_text", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.TextInput.CursorText }},
	{"blurred_textinput_placeholder", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.TextInput.Placeholder }},
	{"blurred_textinput_prompt", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.TextInput.Prompt }},
	{"blurred_textinput_text", func(s *huh.Styles) *lipgloss.Style { return &s.Blurred.TextInput.Text }},

	{"group_title", func(s *huh.Styles) *lipgloss.Style { return &s.Group.Title }},
	{"group_description", func(s *huh.Styles) *lipgloss.Style { return &s.Group.Description }},
}

// knownHuhKeys is the set of huh keys HuhTheme can apply.
var knownHuhKeys = func() map[string]bool {
	m := make(map[string]bool, len(huhTargets))
	for _, ht := range huhTargets {
		m[ht.key] = true
	}
	return m
}()

// -----------------------------------------------------------------------------
// Interactive Helpers
// -----------------------------------------------------------------------------

// ConfirmInput presents a huh confirm prompt using the supplied theme.
func ConfirmInput(title, description string, theme huh.ThemeFunc) (bool, error) {
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative("Yep!").
				Negative("Wait, no").
				Value(&confirm).
				WithTheme(theme),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}
	return confirm, nil
}

// DefaultListBullet is the glyph used by LipglossList.
const DefaultListBullet = "\U000f1978" // "nf-md-dots_circle"

// LipglossList renders a bulleted list using the supplied lipgloss style.
func LipglossList(gloss lipgloss.Style, items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = gloss.Render(DefaultListBullet, s)
	}
	return strings.Join(parts, "\n")
}

// -----------------------------------------------------------------------------
// File Loading
// -----------------------------------------------------------------------------

// LoadThemeConfig reads, strictly decodes, and validates a theme.json file into
// a ThemeConfig. Unknown keys (top-level, style fields, and huh keys) and
// invalid palette references are rejected with an error naming the offending
// key. A missing file is returned as an error so callers can decide whether to
// fall back to defaults.
func LoadThemeConfig(path string) (*ThemeConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("theme file path is empty")
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("load theme %q: %w", path, err)
	}

	var cfg ThemeConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("theme file %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("theme file %q: %w", path, err)
	}
	return &cfg, nil
}

// Validate reports whether the config uses only recognized keys and colors.
// Palette values must be literal colors (hex, ANSI 256 index, ANSI name,
// "none", or empty). Style and huh color fields must reference a palette entry
// (from this config or the built-in defaults) or a literal color. Since
// NewTheme merges over DefaultThemeConfig, the default palette names are
// accepted here as well.
func (c *ThemeConfig) Validate() error {
	if err := c.validatePalette(); err != nil {
		return err
	}

	knownColors := map[string]bool{}
	for name := range DefaultThemeConfig().Palette {
		knownColors[name] = true
	}
	for name := range c.Palette {
		knownColors[name] = true
	}

	for name, def := range c.Styles {
		if err := validateStyleDef(def, knownColors); err != nil {
			return fmt.Errorf("style %q: %w", name, err)
		}
	}

	for key, def := range c.Huh {
		if !knownHuhKeys[key] {
			return fmt.Errorf("unknown huh key %q", key)
		}
		if err := validateStyleDef(def, knownColors); err != nil {
			return fmt.Errorf("huh %q: %w", key, err)
		}
	}

	return nil
}

func (c *ThemeConfig) validatePalette() error {
	for name, spec := range c.Palette {
		if spec.IsAdaptive() {
			for _, v := range []string{spec.Light, spec.Dark} {
				if !isLiteralColor(v) {
					return fmt.Errorf("palette %q: unknown color %q", name, v)
				}
			}
			continue
		}
		if !isLiteralColor(spec.Raw) {
			return fmt.Errorf("palette %q: unknown color %q", name, spec.Raw)
		}
	}
	return nil
}

func validateStyleDef(def StyleDef, knownColors map[string]bool) error {
	fields := []struct {
		name  string
		value string
	}{
		{"foreground", def.Foreground},
		{"background", def.Background},
		{"border_foreground", def.BorderForeground},
	}
	for _, f := range fields {
		v := strings.TrimSpace(f.value)
		if v == "" {
			continue
		}
		if knownColors[v] || isLiteralColor(v) {
			continue
		}
		return fmt.Errorf("unknown color reference %q in field %q", f.value, f.name)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Comparing against the defaults
// -----------------------------------------------------------------------------

// DiffStatus classifies a single config key relative to the built-in defaults.
type DiffStatus string

const (
	// DiffDefault means the key is absent from the user config, so the
	// built-in default applies.
	DiffDefault DiffStatus = "default"
	// DiffRedundant means the key is present in the user config but identical
	// to the built-in default, so it can be removed with no behavior change.
	DiffRedundant DiffStatus = "redundant"
	// DiffOverride means the key is present and differs from the built-in
	// default (or has no default at all, for user-added styles).
	DiffOverride DiffStatus = "override"
)

// KeyDiff describes how one config key relates to the built-in defaults.
type KeyDiff struct {
	// Section is one of "palette", "styles", or "huh".
	Section string
	// Key is the palette name, style name, or huh key.
	Key string
	// Status classifies the key.
	Status DiffStatus
	// DroppedFields lists default fields that a wholesale override no longer
	// sets, so the effective style loses them. It is only populated for
	// "styles" and "huh" overrides.
	DroppedFields []string
}

// String renders the diff as a single human-readable line.
func (d KeyDiff) String() string {
	s := fmt.Sprintf("%s.%s: %s", d.Section, d.Key, d.Status)
	if len(d.DroppedFields) > 0 {
		s += fmt.Sprintf(" (drops default fields: %s)", strings.Join(d.DroppedFields, ", "))
	}
	return s
}

// DiffDefaults compares the config against DefaultThemeConfig and reports, for
// every palette entry, style, and huh key, whether the user config leaves it at
// the default, repeats the default verbatim (redundant), or overrides it.
//
// The result is sorted by section then key. For overrides of styles and huh
// keys it also lists any default fields the override no longer sets, which is
// the wholesale-replace trap: a user "highlight" that omits "bold" is reported
// as an override that drops "bold".
//
// DiffDefaults does not validate; pair it with Validate to catch unknown keys.
func (c *ThemeConfig) DiffDefaults() []KeyDiff {
	if c == nil {
		c = &ThemeConfig{}
	}
	def := DefaultThemeConfig()

	var diffs []KeyDiff
	diffs = append(diffs, diffPalette(def.Palette, c.Palette)...)
	diffs = append(diffs, diffStyles("styles", def.Styles, c.Styles)...)
	diffs = append(diffs, diffStyles("huh", def.Huh, c.Huh)...)
	return diffs
}

func diffPalette(def, user map[string]ColorSpec) []KeyDiff {
	var diffs []KeyDiff
	for _, name := range unionKeys(def, user) {
		d := KeyDiff{Section: "palette", Key: name}
		u, ok := user[name]
		switch {
		case !ok:
			d.Status = DiffDefault
		case u == def[name]:
			d.Status = DiffRedundant
		default:
			d.Status = DiffOverride
		}
		diffs = append(diffs, d)
	}
	return diffs
}

func diffStyles(section string, def, user map[string]StyleDef) []KeyDiff {
	var diffs []KeyDiff
	for _, name := range unionKeys(def, user) {
		d := KeyDiff{Section: section, Key: name}
		u, ok := user[name]
		switch {
		case !ok:
			d.Status = DiffDefault
		case u == def[name]:
			d.Status = DiffRedundant
		default:
			d.Status = DiffOverride
			d.DroppedFields = droppedStyleFields(def[name], u)
		}
		diffs = append(diffs, d)
	}
	return diffs
}

// unionKeys returns the sorted union of the keys of two maps.
func unionKeys[V any](a, b map[string]V) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}

// droppedStyleFields returns the names of fields set in def but not in user.
func droppedStyleFields(def, user StyleDef) []string {
	var dropped []string
	add := func(name string, present bool) {
		if present {
			dropped = append(dropped, name)
		}
	}
	add("foreground", def.Foreground != "" && user.Foreground == "")
	add("background", def.Background != "" && user.Background == "")
	add("border_foreground", def.BorderForeground != "" && user.BorderForeground == "")
	add("bold", def.Bold && !user.Bold)
	add("italic", def.Italic && !user.Italic)
	add("underline", def.Underline && !user.Underline)
	add("strikethrough", def.Strikethrough && !user.Strikethrough)
	add("faint", def.Faint && !user.Faint)
	add("blink", def.Blink && !user.Blink)
	add("reverse", def.Reverse && !user.Reverse)
	return dropped
}

// -----------------------------------------------------------------------------
// Internal
// -----------------------------------------------------------------------------

// DefaultThemeConfig returns the built-in default theme configuration.
func DefaultThemeConfig() *ThemeConfig {
	return &ThemeConfig{
		Palette: map[string]ColorSpec{
			"primary":          {Light: "#FF80AB", Dark: "#FF4081"},
			"primary_accent":   {Light: "#FF5FAF", Dark: "#FE5F86"},
			"secondary":        {Light: "#c5adf9", Dark: "#7d56f4"},
			"secondary_accent": {Light: "#64FCDA", Dark: "#04b575"},
			"highlight":        {Light: "#f5d76e", Dark: "#ffd640"},
			"error":            {Raw: "#ff5c57"},
			"text":             {Light: "#14121a", Dark: "#f5f1fa"},
		},
		Styles: map[string]StyleDef{
			"primary":          {Foreground: "primary"},
			"primary_accent":   {Foreground: "primary_accent"},
			"secondary":        {Foreground: "secondary"},
			"secondary_accent": {Foreground: "secondary_accent"},
			"highlight":        {Foreground: "highlight", Bold: true},
			"error":            {Foreground: "error", Bold: true},
			"text":             {Foreground: "text"},
			"dimmed":           {Foreground: "243"},
			"help":             {Foreground: "240"},
		},
		Huh: map[string]StyleDef{
			"focused_title":           {Foreground: "primary", Bold: true},
			"focused_selected_option": {Foreground: "secondary_accent"},
			"focused_description":     {Foreground: "243", Italic: true},
			"blurred_description":     {Foreground: "243", Italic: true},
		},
	}
}

// mergeThemeConfig returns a fresh config with override merged over base.
// Merging is per name and replaces wholesale: palette entries, styles, and huh
// keys from override replace the base entry of the same name. The base maps are
// copied so the result never aliases shared default state.
func mergeThemeConfig(base, override *ThemeConfig) *ThemeConfig {
	merged := &ThemeConfig{
		Palette: maps.Clone(base.Palette),
		Styles:  maps.Clone(base.Styles),
		Huh:     maps.Clone(base.Huh),
	}
	if override == nil {
		return merged
	}
	maps.Copy(merged.Palette, override.Palette)
	maps.Copy(merged.Styles, override.Styles)
	maps.Copy(merged.Huh, override.Huh)
	return merged
}

// isLiteralColor reports whether v is a recognized literal color: a hex value,
// an ANSI 256 index, a known ANSI name, "none", or empty. Unlike
// parseLiteralColor, which accepts any string as a lipgloss color, this rejects
// typos so validation can report them.
func isLiteralColor(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))

	switch v {
	case "", "none",
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"brightblack", "bright_black", "gray", "grey",
		"brightred", "bright_red", "brightgreen", "bright_green",
		"brightyellow", "bright_yellow", "brightblue", "bright_blue",
		"brightmagenta", "bright_magenta", "brightcyan", "bright_cyan",
		"brightwhite", "bright_white":
		return true
	}

	if strings.HasPrefix(v, "#") {
		return isHexColor(v)
	}

	if n, err := strconv.Atoi(v); err == nil {
		return n >= 0 && n <= 255
	}

	return false
}

// isHexColor reports whether v is a "#"-prefixed hex color with 3, 4, 6, or 8
// digits.
func isHexColor(v string) bool {
	if len(v) < 2 || v[0] != '#' {
		return false
	}
	digits := v[1:]
	switch len(digits) {
	case 3, 4, 6, 8:
	default:
		return false
	}
	for _, r := range digits {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func (t *Theme) resolveColorSpec(spec ColorSpec) color.Color {
	if spec.IsAdaptive() {
		light := parseLiteralColor(spec.Light)
		dark := parseLiteralColor(spec.Dark)
		return lipgloss.LightDark(t.hasDarkBg)(light, dark)
	}
	return parseLiteralColor(spec.Raw)
}

func parseLiteralColor(v string) color.Color {
	v = strings.ToLower(strings.TrimSpace(v))

	switch v {
	case "black":
		return lipgloss.Black
	case "red":
		return lipgloss.Red
	case "green":
		return lipgloss.Green
	case "yellow":
		return lipgloss.Yellow
	case "blue":
		return lipgloss.Blue
	case "magenta":
		return lipgloss.Magenta
	case "cyan":
		return lipgloss.Cyan
	case "white":
		return lipgloss.White
	case "brightblack", "bright_black", "gray", "grey":
		return lipgloss.BrightBlack
	case "brightred", "bright_red":
		return lipgloss.BrightRed
	case "brightgreen", "bright_green":
		return lipgloss.BrightGreen
	case "brightyellow", "bright_yellow":
		return lipgloss.BrightYellow
	case "brightblue", "bright_blue":
		return lipgloss.BrightBlue
	case "brightmagenta", "bright_magenta":
		return lipgloss.BrightMagenta
	case "brightcyan", "bright_cyan":
		return lipgloss.BrightCyan
	case "brightwhite", "bright_white":
		return lipgloss.BrightWhite
	case "none", "":
		return nil
	}

	return lipgloss.Color(v)
}

func (t *Theme) parseColorValue(v string) color.Color {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "none") {
		return nil
	}
	if c, ok := t.colors[v]; ok {
		return c
	}
	return parseLiteralColor(v)
}

func (t *Theme) buildStyle(def StyleDef) lipgloss.Style {
	return t.overlayStyleDef(lipgloss.NewStyle(), def)
}

func (t *Theme) overlayStyleDef(base lipgloss.Style, def StyleDef) lipgloss.Style {
	s := base
	if def.Foreground != "" {
		s = s.Foreground(t.parseColorValue(def.Foreground))
	}
	if def.Background != "" {
		s = s.Background(t.parseColorValue(def.Background))
	}
	if def.BorderForeground != "" {
		s = s.BorderForeground(t.parseColorValue(def.BorderForeground))
	}
	if def.Bold {
		s = s.Bold(true)
	}
	if def.Italic {
		s = s.Italic(true)
	}
	if def.Underline {
		s = s.Underline(true)
	}
	if def.Strikethrough {
		s = s.Strikethrough(true)
	}
	if def.Faint {
		s = s.Faint(true)
	}
	if def.Blink {
		s = s.Blink(true)
	}
	if def.Reverse {
		s = s.Reverse(true)
	}
	return s
}
