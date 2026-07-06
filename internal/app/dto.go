package app

type TreeNodeDTO struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	Depth    int    `json:"depth"`
	Expanded bool   `json:"expanded"`
	Selected bool   `json:"selected"`
}

type SearchResultDTO struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Text string `json:"text"`
}

type ThemeDTO struct {
	Background    string `json:"background"`
	Foreground    string `json:"foreground"`
	Border        string `json:"border"`
	BorderFocused string `json:"borderFocused"`
	Accent        string `json:"accent"`
	Muted         string `json:"muted"`
	Success       string `json:"success"`
	Error         string `json:"error"`
	Warning       string `json:"warning"`
}
