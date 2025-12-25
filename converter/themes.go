// Package converter provides Markdown to MediaWiki conversion with configurable themes.
// It converts standard Markdown syntax to MediaWiki markup with optional branding colors.
package converter

// Theme defines color scheme for MediaWiki output
type Theme struct {
	Name        string                  // Theme identifier
	Description string                  // Human-readable description
	Headings    map[int]string          // Heading level -> color (1-6)
	InlineCode  InlineCodeStyle         // Inline code styling
	CodeBlock   CodeBlockStyle          // Code block styling
	Callouts    map[string]CalloutStyle // Callout type -> styling
}

// InlineCodeStyle defines styling for inline code (`code`)
type InlineCodeStyle struct {
	BackgroundColor string
	TextColor       string
	Padding         string
	BorderRadius    string
	FontFamily      string
}

// CodeBlockStyle defines styling for fenced code blocks
type CodeBlockStyle struct {
	BackgroundColor string
	BorderColor     string
	BorderLeftColor string
	FontFamily      string
}

// CalloutStyle defines styling for callout boxes (> [!NOTE], etc.)
type CalloutStyle struct {
	Emoji       string
	Label       string
	BorderColor string
	BgColor     string
	TextColor   string
}

// Pre-defined themes

// ThemeTieto applies Tieto brand colors (Hero Blue #021e57)
var ThemeTieto = Theme{
	Name:        "tieto",
	Description: "Tieto brand colors with Hero Blue headings and yellow code highlights",
	Headings: map[int]string{
		1: "#021e57", // Hero Blue
		2: "#021e57",
		3: "#021e57",
		4: "#021e57",
		5: "#021e57",
		6: "#021e57",
	},
	InlineCode: InlineCodeStyle{
		BackgroundColor: "#f5ff56", // Bright yellow (Tieto accent)
		TextColor:       "#021e57", // Hero Blue
		Padding:         "2px 6px",
		BorderRadius:    "3px",
		FontFamily:      "Consolas,Monaco,monospace",
	},
	CodeBlock: CodeBlockStyle{
		BackgroundColor: "#FAFAFA",
		BorderColor:     "#CCCCCC",
		BorderLeftColor: "#021e57", // Hero Blue accent
		FontFamily:      "'Consolas', 'Monaco', 'Courier New', monospace",
	},
	Callouts: map[string]CalloutStyle{
		"note":      {"📝", "Note", "#839df9", "#f7f7fa", "#071d49"},
		"info":      {"ℹ️", "Info", "#021e57", "#f7f7fa", "#021e57"},
		"tip":       {"💡", "Tip", "#4e60e7", "#f7f7fa", "#071d49"},
		"warning":   {"⚠️", "Warning", "#e6a700", "#fff8e6", "#8a6500"},
		"caution":   {"🔶", "Caution", "#e65c00", "#fff0e6", "#8a3800"},
		"important": {"❗", "Important", "#d63384", "#fdf2f8", "#9d174d"},
		"success":   {"✅", "Success", "#4e60e7", "#f7f7fa", "#071d49"},
	},
}

// ThemeNeutral provides clean conversion without brand colors
var ThemeNeutral = Theme{
	Name:        "neutral",
	Description: "Clean MediaWiki output without custom colors or branding",
	Headings:    map[int]string{}, // Empty = no color styling
	InlineCode: InlineCodeStyle{
		BackgroundColor: "#f4f4f4",
		TextColor:       "#333333",
		Padding:         "2px 4px",
		BorderRadius:    "3px",
		FontFamily:      "monospace",
	},
	CodeBlock: CodeBlockStyle{
		BackgroundColor: "#f8f8f8",
		BorderColor:     "#ddd",
		BorderLeftColor: "#ccc",
		FontFamily:      "monospace",
	},
	Callouts: map[string]CalloutStyle{
		"note":      {"📝", "Note", "#0066cc", "#f0f7ff", "#003366"},
		"info":      {"ℹ️", "Info", "#0066cc", "#f0f7ff", "#003366"},
		"tip":       {"💡", "Tip", "#28a745", "#f0f9f4", "#155724"},
		"warning":   {"⚠️", "Warning", "#ffc107", "#fff8e6", "#856404"},
		"caution":   {"🔶", "Caution", "#fd7e14", "#fff0e6", "#8a3800"},
		"important": {"❗", "Important", "#dc3545", "#fdf2f2", "#721c24"},
		"success":   {"✅", "Success", "#28a745", "#f0f9f4", "#155724"},
	},
}

// ThemeDark provides dark-mode friendly styling
var ThemeDark = Theme{
	Name:        "dark",
	Description: "Dark mode optimized colors for wikis with dark themes",
	Headings: map[int]string{
		1: "#7cb3ff",
		2: "#7cb3ff",
		3: "#7cb3ff",
		4: "#7cb3ff",
		5: "#7cb3ff",
		6: "#7cb3ff",
	},
	InlineCode: InlineCodeStyle{
		BackgroundColor: "#2d2d2d",
		TextColor:       "#e6e6e6",
		Padding:         "2px 4px",
		BorderRadius:    "3px",
		FontFamily:      "monospace",
	},
	CodeBlock: CodeBlockStyle{
		BackgroundColor: "#1e1e1e",
		BorderColor:     "#444",
		BorderLeftColor: "#7cb3ff",
		FontFamily:      "monospace",
	},
	Callouts: map[string]CalloutStyle{
		"note":      {"📝", "Note", "#5c9aff", "#1a2744", "#a8c7ff"},
		"info":      {"ℹ️", "Info", "#5c9aff", "#1a2744", "#a8c7ff"},
		"tip":       {"💡", "Tip", "#4ade80", "#1a3328", "#86efac"},
		"warning":   {"⚠️", "Warning", "#fbbf24", "#3d3214", "#fcd34d"},
		"caution":   {"🔶", "Caution", "#fb923c", "#3d2814", "#fdba74"},
		"important": {"❗", "Important", "#f87171", "#3d1a1a", "#fca5a5"},
		"success":   {"✅", "Success", "#4ade80", "#1a3328", "#86efac"},
	},
}

// AvailableThemes lists all built-in themes
var AvailableThemes = map[string]Theme{
	"tieto":   ThemeTieto,
	"neutral": ThemeNeutral,
	"dark":    ThemeDark,
}

// GetTheme returns a theme by name, defaulting to neutral if not found
func GetTheme(name string) Theme {
	if theme, ok := AvailableThemes[name]; ok {
		return theme
	}
	return ThemeNeutral
}

// ListThemes returns a list of available theme names with descriptions
func ListThemes() []ThemeInfo {
	themes := make([]ThemeInfo, 0, len(AvailableThemes))
	for name, theme := range AvailableThemes {
		themes = append(themes, ThemeInfo{
			Name:        name,
			Description: theme.Description,
		})
	}
	return themes
}

// ThemeInfo provides basic info about a theme
type ThemeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
