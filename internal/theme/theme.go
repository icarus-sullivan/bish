package theme

type Theme struct {
	Background    string
	Foreground    string
	Border        string
	BorderFocused string
	Accent        string
	Muted         string
	Success       string
	Error         string
	Warning       string
}

func Get(name string) Theme {
	switch name {
	case "light":
		return Theme{
			Background:    "#ffffff",
			Foreground:    "#000000",
			Border:        "#000000",
			BorderFocused: "#333333",
			Accent:        "#0055cc",
			Muted:         "#666666",
			Success:       "#007700",
			Error:         "#cc0000",
			Warning:       "#aa5500",
		}
	case "catppuccin":
		return Theme{
			Background:    "#1e1e2e",
			Foreground:    "#cdd6f4",
			Border:        "#313244",
			BorderFocused: "#cba6f7",
			Accent:        "#cba6f7",
			Muted:         "#585b70",
			Success:       "#a6e3a1",
			Error:         "#f38ba8",
			Warning:       "#fab387",
		}
	case "obsidian":
		return Theme{
			Background:    "#0a0a0a",
			Foreground:    "#d8d0c0",
			Border:        "#1c1c1c",
			BorderFocused: "#5c3fd3",
			Accent:        "#5c3fd3",
			Muted:         "#3c3830",
			Success:       "#4caf7d",
			Error:         "#e05c5c",
			Warning:       "#c9a227",
		}
	case "vos":
		return Theme{
			Background:    "#1e1e1e",
			Foreground:    "#d4d4d4",
			Border:        "#3c3c3c",
			BorderFocused: "#007acc",
			Accent:        "#569cd6",
			Muted:         "#6a6a6a",
			Success:       "#608b4e",
			Error:         "#f44747",
			Warning:       "#ce9178",
		}
	case "monokai":
		return Theme{
			Background:    "#272822",
			Foreground:    "#f8f8f2",
			Border:        "#3e3d32",
			BorderFocused: "#a6e22e",
			Accent:        "#a6e22e",
			Muted:         "#75715e",
			Success:       "#a6e22e",
			Error:         "#f92672",
			Warning:       "#fd971f",
		}
	case "gruvbox":
		return Theme{
			Background:    "#282828",
			Foreground:    "#ebdbb2",
			Border:        "#504945",
			BorderFocused: "#fabd2f",
			Accent:        "#fabd2f",
			Muted:         "#928374",
			Success:       "#b8bb26",
			Error:         "#fb4934",
			Warning:       "#fe8019",
		}
	case "nord":
		return Theme{
			Background:    "#2e3440",
			Foreground:    "#eceff4",
			Border:        "#3b4252",
			BorderFocused: "#88c0d0",
			Accent:        "#88c0d0",
			Muted:         "#4c566a",
			Success:       "#a3be8c",
			Error:         "#bf616a",
			Warning:       "#ebcb8b",
		}
	case "tokyo-night":
		return Theme{
			Background:    "#1a1b26",
			Foreground:    "#a9b1d6",
			Border:        "#24283b",
			BorderFocused: "#7aa2f7",
			Accent:        "#7aa2f7",
			Muted:         "#414868",
			Success:       "#9ece6a",
			Error:         "#f7768e",
			Warning:       "#e0af68",
		}
	default: // "void" — deep dark with electric blue
		return Theme{
			Background:    "#0d0f17",
			Foreground:    "#d4d8ed",
			Border:        "#1e2133",
			BorderFocused: "#4d9ef7",
			Accent:        "#7986cb",
			Muted:         "#3a3f5c",
			Success:       "#4dc988",
			Error:         "#ff5f6e",
			Warning:       "#ffa040",
		}
	}
}
