<p align="center">
  <source media="(prefers-color-scheme: dark)" srcset="images/tint-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="images/tint-light.png">
  <img alt="Project Logo" src="images/tint-dark.png" width="128">
</p>

# tint

Opinionated theme loader for Go CLI/TUI projects built on
[charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) and
[charmbracelet/huh](https://github.com/charmbracelet/huh).

## Install

```sh
go get go.fuzzyporpoise.dev/tint
```

## Usage

```go
package main

import (
    "fmt"

    "go.fuzzyporpoise.dev/tint"
)

func main() {
    cfg, err := tint.LoadThemeConfig("theme.json")
    if err != nil {
        // Missing files fall back to defaults.
        cfg = nil
    }

    th := tint.NewTheme(cfg)
    fmt.Println(th.Style("highlight").Render("hello"))
}
```

## theme.json

```json
{
  "palette": {
    "primary": { "light": "#FF80AB", "dark": "#FF4081" },
    "error": { "color": "#ff5c57" }
  },
  "styles": {
    "highlight": { "foreground": "primary", "bold": true }
  },
  "huh": {
    "focused_title": { "foreground": "primary", "bold": true }
  }
}
```

`color` can be a hex string, an ANSI 256 index, an ANSI colour name, or an
adaptive object with `light` and `dark` values.

### Overlaying the defaults

`theme.json` is merged over the built-in defaults, so you only need to specify
what you want to change. Unspecified palette entries, styles, and huh keys keep
their default values, and a missing file falls back to the defaults entirely.

Merging is per name and replaces wholesale, not field by field. A `highlight`
entry replaces the default `highlight` style completely, so restate the whole
style (including any flags you want to keep):

```json
{
  "styles": {
    "highlight": { "foreground": "highlight", "bold": true }
  }
}
```

### Validation

Unknown keys are rejected with an error rather than silently ignored. This
covers unknown top-level keys, unknown style fields (for example a misspelled
`"foregound"`), unknown `huh` keys, and style color values that are neither a
palette name nor a recognized literal color. A missing file is still returned
as an error (`os.ErrNotExist`), so callers can choose to fall back to defaults.

### Default theme

This is the built-in configuration that `theme.json` overlays. Anything you
omit is taken from here, and the export `tint.DefaultThemeConfig()` returns the
same values in code.

Palette:

| name               | value                                             |
| ------------------ | ------------------------------------------------- |
| `primary`          | light `#FF80AB`, dark `#FF4081`                   |
| `primary_accent`   | light `#FF5FAF`, dark `#FE5F86`                   |
| `secondary`        | light `#c5adf9`, dark `#7d56f4`                   |
| `secondary_accent` | light `#64FCDA`, dark `#04b575`                   |
| `highlight`        | light `#f5d76e`, dark `#ffd640`                   |
| `error`            | `#ff5c57`                                         |
| `text`             | light `#14121a`, dark `#f5f1fa`                   |

Styles:

| name               | definition                                 |
| ------------------ | ------------------------------------------ |
| `primary`          | foreground `primary`                       |
| `primary_accent`   | foreground `primary_accent`                |
| `secondary`        | foreground `secondary`                     |
| `secondary_accent` | foreground `secondary_accent`              |
| `highlight`        | foreground `highlight`, bold               |
| `error`            | foreground `error`, bold                   |
| `text`             | foreground `text`                          |
| `dimmed`           | foreground `243`                           |
| `help`             | foreground `240`                           |

Huh keys:

| key                       | definition                        |
| ------------------------- | --------------------------------- |
| `focused_title`           | foreground `primary`, bold        |
| `focused_selected_option` | foreground `secondary_accent`     |
| `focused_description`     | foreground `243`, italic          |
| `blurred_description`     | foreground `243`, italic          |

### Checking your file against the defaults

Because a config is merged over the defaults, a key can be redundant (it repeats
the default), a real override, or absent (the default applies). Drop the program
below into a file, point `themePath` at your theme, and run it with
`go run check-theme.go`. It validates the file and then prints one line per key:

```go
package main

import (
	"errors"
	"fmt"
	"os"

	"go.fuzzyporpoise.dev/tint"
)

func main() {
	themePath := "theme.json"

	cfg, err := tint.LoadThemeConfig(themePath)
	switch {
	case err == nil:
		// parsed and validated
	case errors.Is(err, os.ErrNotExist):
		fmt.Printf("%s not found; every key falls back to the default\n\n", themePath)
		cfg = &tint.ThemeConfig{}
	default:
		fmt.Fprintf(os.Stderr, "invalid theme: %v\n", err)
		os.Exit(1)
	}

	for _, d := range cfg.DiffDefaults() {
		fmt.Println(d)
	}
}
```

Example output:

```text
palette.error: default
palette.primary: redundant
huh.focused_title: default
styles.highlight: override (drops default fields: bold)
styles.help: default
```

`DiffDefault` means the key is absent, `DiffRedundant` means it can be removed
with no behavior change, and `DiffOverride` means it changes something.
`DroppedFields` lists default fields a wholesale override no longer sets, so
`styles.highlight: override (drops default fields: bold)` is your reminder to
restate `"bold": true` if you wanted to keep it. `LoadThemeConfig` already
validates, so unknown keys fail before the diff is printed.

## Huh forms

```go
form := huh.NewForm(
    huh.NewGroup(
        huh.NewInput().Title("Name").Value(&name),
    ),
).WithTheme(th.HuhTheme(true))
```

## Development

```sh
make check
```
