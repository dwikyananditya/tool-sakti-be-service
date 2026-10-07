// Package dsm parses Daily Stand-up Meeting notes into per-ticket entries.
package dsm

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"tool-sakti-be-service/internal/github"
)

type Entry struct {
	Assignee    string
	Date        string
	Session     string
	TicketURL   string
	TicketTitle string
	Status      string
	Line        int
	Raw         string
}

type DailyEntry struct {
	Assignee                 string
	Date                     string
	Session                  string
	TicketURL                string
	TicketTitle              string
	Status                   string
	Sessions                 []string
	Occurrences              int
	FirstSession             string
	LastSession              string
	ContinuedFromPreviousDay bool
}

var months = map[string]int{
	"january": 0, "february": 1, "march": 2, "april": 3, "may": 4, "june": 5,
	"july": 6, "august": 7, "september": 8, "october": 9, "november": 10, "december": 11,
	"januari": 0, "februari": 1, "maret": 2, "mei": 4, "juni": 5, "juli": 6,
	"agustus": 7, "oktober": 9, "desember": 11,
}

var aliases = []struct {
	name    string
	aliases []string
}{
	{"allief", []string{"allief", "allif", "allifgobimbel"}},
	{"hizkia", []string{"hizkia", "hizkiagobimbel"}},
	{"maulana", []string{"maulana", "maulanagobimbel"}},
	{"dwiki", []string{"dwiki", "dwiky", "dwikigobimbel", "dwikygobimbel"}},
}

func AllAssignees() []string {
	out := make([]string, len(aliases))
	for i, a := range aliases {
		out[i] = a.name
	}
	return out
}

func pad(v int) string { return fmt.Sprintf("%02d", v) }

var (
	numericDateRe = regexp.MustCompile(`\b(\d{1,2})[/-](\d{1,2})[/-](\d{4})\b`)
	namedDateRe   = regexp.MustCompile(`\b(\d{1,2})\s+([a-z]+)\s*(\d{4})?\b`)
	sessionRe     = regexp.MustCompile(`(?i)\b(?:pukul|jam|dsm|sesi)?\s*(11|15|16)(?:[:.]([0-5]\d))?\b`)
	titleRe       = regexp.MustCompile(`(?i)(?:#{1,6}\s*)?(?:\*{1,2})?Task\s+\d+\s*\|\s*(.+?)(?:\*{1,2})?\s*$`)
	statusRe      = regexp.MustCompile(`(?i)^Status\s*:\s*(.+)$`)
	urlRe         = regexp.MustCompile(`(?i)https://github\.com/[\w.-]+/[\w.-]+/(?:issues|pull)/\d+`)
)

func ParseDateFromText(text string, yearHint int) string {
	if m := numericDateRe.FindStringSubmatch(text); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		return fmt.Sprintf("%s-%s-%s", m[3], pad(mo), pad(d))
	}
	if m := namedDateRe.FindStringSubmatch(strings.ToLower(text)); m != nil {
		if mo, ok := months[m[2]]; ok {
			d, _ := strconv.Atoi(m[1])
			year := m[3]
			if year == "" {
				year = strconv.Itoa(yearHint)
			}
			return fmt.Sprintf("%s-%s-%s", year, pad(mo+1), pad(d))
		}
	}
	return ""
}

func ParseSessionFromText(text string) string {
	m := sessionRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	min := m[2]
	if min == "" {
		min = "00"
	}
	return m[1] + ":" + min
}

func findAssignees(text string) []string {
	lower := strings.ToLower(text)
	var out []string
	for _, a := range aliases {
		for _, alias := range a.aliases {
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\b`).MatchString(lower) {
				out = append(out, a.name)
				break
			}
		}
	}
	return out
}

func ParseDsm(markdown string, year int, selectedAssignees []string) []Entry {
	if year == 0 {
		year = 2026
	}
	selected := map[string]bool{}
	if len(selectedAssignees) == 0 {
		for _, n := range AllAssignees() {
			selected[n] = true
		}
	} else {
		for _, v := range selectedAssignees {
			selected[strings.ToLower(v)] = true
		}
	}

	lines := regexp.MustCompile(`\r?\n`).Split(markdown, -1)
	var entries []Entry
	var currentDate, currentSession, currentTitle string
	var currentAssignees []string
	var currentEntryIndexes []int

	for lineIndex, line := range lines {
		if d := ParseDateFromText(line, year); d != "" {
			currentDate = d
		}
		if s := ParseSessionFromText(line); s != "" {
			currentSession = s
		}
		lineAssignees := findAssignees(line)
		if len(lineAssignees) > 0 {
			currentAssignees = lineAssignees
		}

		if m := titleRe.FindStringSubmatch(line); m != nil {
			currentTitle = cleanMarkdown(m[1])
			currentEntryIndexes = nil
		}

		if m := statusRe.FindStringSubmatch(cleanMarkdown(line)); m != nil {
			status := strings.TrimSpace(m[1])
			for _, idx := range currentEntryIndexes {
				entries[idx].Status = status
			}
		}

		normalizedLine := strings.ReplaceAll(line, `https\://`, "https://")
		seen := map[string]bool{}
		for _, raw := range urlRe.FindAllString(normalizedLine, -1) {
			norm := github.NormalizeURL(raw)
			if norm == "" || seen[norm] {
				continue
			}
			seen[norm] = true

			assignees := lineAssignees
			if len(assignees) == 0 {
				assignees = currentAssignees
			}
			for _, assignee := range assignees {
				if !selected[assignee] {
					continue
				}
				entries = append(entries, Entry{
					Assignee:    assignee,
					Date:        currentDate,
					Session:     currentSession,
					TicketURL:   norm,
					TicketTitle: currentTitle,
					Line:        lineIndex + 1,
					Raw:         strings.TrimSpace(line),
				})
				currentEntryIndexes = append(currentEntryIndexes, len(entries)-1)
			}
		}
	}

	out := entries[:0]
	for _, e := range entries {
		if e.Date != "" && e.Session != "" {
			out = append(out, e)
		}
	}
	return out
}

var (
	mdEscapeRe = regexp.MustCompile(`\\([_\[\]*:#-])`)
	mdBulletRe = regexp.MustCompile(`^\s*[-*]+\s*`)
	mdStarsRe  = regexp.MustCompile(`\*+`)
	mdSpaceRe  = regexp.MustCompile(`\s+`)
)

func cleanMarkdown(v string) string {
	v = mdEscapeRe.ReplaceAllString(v, "$1")
	v = mdBulletRe.ReplaceAllString(v, "")
	v = mdStarsRe.ReplaceAllString(v, "")
	v = mdSpaceRe.ReplaceAllString(v, " ")
	return strings.TrimSpace(v)
}

var sessionMinutesRe = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)

func sessionMinutes(session string) int {
	m := sessionMinutesRe.FindStringSubmatch(session)
	if m == nil {
		return math.MaxInt32
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	return h*60 + mi
}

func CollapseDailyEntries(entries []Entry) []DailyEntry {
	type group struct {
		key   string
		items []Entry
	}
	order := []string{}
	byKey := map[string][]Entry{}
	for _, e := range entries {
		key := e.Assignee + "|" + e.TicketURL + "|" + e.Date
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], e)
	}

	var out []DailyEntry
	for _, key := range order {
		items := append([]Entry(nil), byKey[key]...)
		sort.SliceStable(items, func(i, j int) bool {
			return sessionMinutes(items[i].Session) < sessionMinutes(items[j].Session)
		})

		latestStatus := ""
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].Status != "" {
				latestStatus = items[i].Status
				break
			}
		}
		latestTitle := ""
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].TicketTitle != "" {
				latestTitle = items[i].TicketTitle
				break
			}
		}
		if latestTitle == "" {
			latestTitle = items[0].TicketTitle
		}

		var sessions []string
		for _, it := range items {
			if it.Session != "" {
				sessions = append(sessions, it.Session)
			}
		}

		out = append(out, DailyEntry{
			Assignee:     items[0].Assignee,
			Date:         items[0].Date,
			Session:      items[0].Session,
			TicketURL:    items[0].TicketURL,
			TicketTitle:  latestTitle,
			Status:       latestStatus,
			Sessions:     sessions,
			Occurrences:  len(items),
			FirstSession: items[0].Session,
			LastSession:  items[len(items)-1].Session,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		a := out[i].Date + "|" + out[i].Assignee + "|" + out[i].TicketURL
		b := out[j].Date + "|" + out[j].Assignee + "|" + out[j].TicketURL
		return a < b
	})
	return out
}
