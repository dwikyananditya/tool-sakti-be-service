package github

import (
	"regexp"
	"strings"
	"time"
)

type Scan struct {
	URL        string
	Title      string
	TargetDate string
	LinkedURLs []string
	Events     []Event
}

type Event struct {
	DateTime     time.Time
	TargetStatus string
	ClosedThis   bool
	WorkActivity bool
}

var statusDashes = regexp.MustCompile(`[-_]+`)
var statusSpaces = regexp.MustCompile(`\s+`)

func NormalizeStatus(v string) string {
	v = strings.ToLower(v)
	v = statusDashes.ReplaceAllString(v, " ")
	v = statusSpaces.ReplaceAllString(v, " ")
	return strings.TrimSpace(v)
}

var jakarta = mustLoadJakarta()

func mustLoadJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}

func JakartaDate(t time.Time) string {
	return t.In(jakarta).Format("2006-01-02")
}

// HasValidEnd checks the events for an end signal. events must already be
// filtered to the relevant Jakarta date.
func HasValidEnd(events []Event, status string) bool {
	norm := NormalizeStatus(status)
	for _, e := range events {
		if e.TargetStatus == norm {
			return true
		}
	}
	if norm == "deployed" {
		for _, e := range events {
			if e.ClosedThis {
				return true
			}
		}
	}
	for _, e := range events {
		if e.WorkActivity {
			return true
		}
	}
	return false
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
