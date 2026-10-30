package shared

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

var SecurityDimensions = []string{"system", "network", "website", "docker", "logs"}
var securityCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,95}$`)
var securityEventKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Options contain only bounded rule parameters; paths, shell and executable names
// are never accepted from the control plane.
type SecurityOptions struct {
	Weights           map[string]int `json:"weights"`
	WindowSeconds     int            `json:"window_seconds"`
	SSHFailures       int            `json:"ssh_failures"`
	Requests          int            `json:"requests"`
	ErrorPercent      int            `json:"error_percent"`
	SensitiveRequests int            `json:"sensitive_requests"`
}

func DefaultSecurityOptions() SecurityOptions {
	return SecurityOptions{map[string]int{"system": 20, "network": 20, "website": 20, "docker": 20, "logs": 20}, 300, 10, 120, 40, 10}
}
func (o SecurityOptions) Valid() bool {
	if len(o.Weights) != 5 || o.WindowSeconds < 60 || o.WindowSeconds > 3600 || o.SSHFailures < 3 || o.SSHFailures > 10000 || o.Requests < 10 || o.Requests > 100000 || o.ErrorPercent < 10 || o.ErrorPercent > 100 || o.SensitiveRequests < 3 || o.SensitiveRequests > 10000 {
		return false
	}
	for _, d := range SecurityDimensions {
		if o.Weights[d] < 1 || o.Weights[d] > 100 {
			return false
		}
	}
	return true
}

type SecurityCheck struct {
	Rule      string `json:"rule"`
	Dimension string `json:"dimension"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Severity  string `json:"severity"`
	Target    string `json:"target"`
	Evidence  string `json:"evidence"`
	Reason    string `json:"reason"`
	Advice    string `json:"advice"`
}
type SecurityObservation struct {
	Key        string `json:"key"`
	Rule       string `json:"rule"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	ObservedAt int64  `json:"observed_at"`
}
type SecurityReport struct {
	Checks     []SecurityCheck       `json:"checks"`
	Events     []SecurityObservation `json:"events"`
	Scope      string                `json:"scope"`
	StartedAt  int64                 `json:"started_at"`
	FinishedAt int64                 `json:"finished_at"`
}
type SecurityGrade struct {
	Score        *int           `json:"score"`
	Risk         string         `json:"risk"`
	Checked      int            `json:"checked"`
	Unknown      int            `json:"unknown"`
	NotInstalled int            `json:"not_installed"`
	Critical     int            `json:"critical"`
	High         int            `json:"high"`
	Medium       int            `json:"medium"`
	Low          int            `json:"low"`
	Dimensions   map[string]int `json:"dimensions"`
}

func SecuritySeverity(s string) bool {
	return s == "info" || s == "low" || s == "medium" || s == "high" || s == "critical"
}
func (r SecurityReport) Valid() bool {
	if len(r.Checks) == 0 || len(r.Checks) > 48 || len(r.Events) > 24 || len(r.Scope) > 300 || r.StartedAt <= 0 || r.FinishedAt < r.StartedAt {
		return false
	}
	seen := map[string]bool{}
	for _, c := range r.Checks {
		if !securityCode.MatchString(c.Rule) || seen[c.Rule] || !SecuritySeverity(c.Severity) || len(c.Title) > 160 || len(c.Target) > 240 || len(c.Evidence) > 700 || len(c.Reason) > 500 || len(c.Advice) > 900 {
			return false
		}
		seen[c.Rule] = true
		if !strings.Contains("|system|network|website|docker|logs|", "|"+c.Dimension+"|") || c.Dimension == "" {
			return false
		}
		switch c.Status {
		case "pass", "risk", "unknown", "error", "not_installed", "not_authorized":
		default:
			return false
		}
	}
	for _, e := range r.Events {
		if len(e.Key) != 64 || !securityEventKey.MatchString(e.Key) || !securityCode.MatchString(e.Rule) || !SecuritySeverity(e.Severity) || len(e.Message) > 700 || e.ObservedAt <= 0 {
			return false
		}
	}
	b, _ := json.Marshal(r)
	return len(b) <= 45000
}

// Unknown and absent services never become passes. Only measured dimensions
// contribute to the normalized score. Workflow labels are deliberately absent.
func GradeSecurity(checks []SecurityCheck, options SecurityOptions) SecurityGrade {
	g := SecurityGrade{Risk: "unknown", Dimensions: map[string]int{}}
	penalty := map[string]int{}
	measured := map[string]bool{}
	for _, c := range checks {
		if c.Status == "not_installed" {
			g.NotInstalled++
			continue
		}
		if c.Status != "pass" && c.Status != "risk" {
			g.Unknown++
			continue
		}
		g.Checked++
		measured[c.Dimension] = true
		if c.Status == "risk" {
			p := 0
			switch c.Severity {
			case "critical":
				g.Critical++
				p = 60
			case "high":
				g.High++
				p = 30
			case "medium":
				g.Medium++
				p = 15
			case "low":
				g.Low++
				p = 5
			}
			penalty[c.Dimension] += p
		}
	}
	total, weights := 0, 0
	for _, d := range SecurityDimensions {
		if measured[d] {
			s := max(0, 100-penalty[d])
			g.Dimensions[d] = s
			total += s * options.Weights[d]
			weights += options.Weights[d]
		}
	}
	if weights > 0 {
		score := total / weights
		g.Score = &score
		g.Risk = "low"
		if score < 80 || g.Medium > 0 {
			g.Risk = "medium"
		}
		if score < 60 || g.High > 0 {
			g.Risk = "high"
		}
		if g.Critical > 0 {
			g.Risk = "critical"
		}
	}
	return g
}
func SecurityText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, RedactOutput(s))
	if len(s) > limit {
		s = s[:limit]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}
