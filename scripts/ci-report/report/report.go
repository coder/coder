package report

type CI struct {
	Packages []Package `json:"packages"`
	Tests    []Test    `json:"tests"`
}

type Package struct {
	Name      string  `json:"name"`
	Time      float64 `json:"time"`
	Skip      bool    `json:"skip,omitempty"`
	Fail      bool    `json:"fail,omitempty"`
	NumFailed int     `json:"num_failed,omitempty"`
	Timeout   bool    `json:"timeout,omitempty"`
	Output    string  `json:"output,omitempty"` // Output present e.g. for timeout.
}

type Test struct {
	Package string  `json:"package"`
	Name    string  `json:"name"`
	Time    float64 `json:"time"`
	Skip    bool    `json:"skip,omitempty"`
	Fail    bool    `json:"fail,omitempty"`
	Timeout bool    `json:"timeout,omitempty"`
	Output  string  `json:"output,omitempty"`
}
