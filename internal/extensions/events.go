package extensions

type Event struct {
	Type    string   `json:"type"`
	Message string   `json:"message"`
	Percent float64  `json:"percent"`
	Paths   []string `json:"paths"`
}
