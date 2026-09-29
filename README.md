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
