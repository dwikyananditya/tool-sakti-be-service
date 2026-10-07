package rekap

import (
	"sort"
	"strings"

	"tool-sakti-be-service/internal/dsm"
	"tool-sakti-be-service/internal/github"
)

type Period struct {
	StartDate string
	EndDate   string
}

func UniqueTicketPeriod(entries []dsm.DailyEntry, scans map[string]*github.Scan) Period {
	if len(entries) == 0 {
		return Period{}
	}
	ordered := append([]dsm.DailyEntry{}, entries...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Date < ordered[j].Date })
	first := ordered[0]

	parent := scans[first.TicketURL]
	var events []github.Event
	if parent != nil {
		events = append(events, parent.Events...)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].DateTime.Before(events[j].DateTime) })

	period := Period{StartDate: first.Date}
	for _, e := range events {
		if e.TargetStatus == startStatus {
			period.StartDate = localDate(e.DateTime)
			break
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.TargetStatus == "deployed" || e.ClosedThis {
			period.EndDate = localDate(e.DateTime)
			break
		}
	}
	return period
}

func TargetDateForTicket(url string, scans map[string]*github.Scan) string {
	if s := scans[url]; s != nil {
		return s.TargetDate
	}
	return ""
}

func LatestGitHubStatus(url string, scans map[string]*github.Scan) string {
	s := scans[url]
	if s == nil {
		return ""
	}
	var statusEvents []github.Event
	for _, e := range s.Events {
		if e.TargetStatus != "" {
			statusEvents = append(statusEvents, e)
		}
	}
	sort.SliceStable(statusEvents, func(i, j int) bool { return statusEvents[i].DateTime.After(statusEvents[j].DateTime) })
	if len(statusEvents) > 0 {
		return statusEvents[0].TargetStatus
	}
	return ""
}

func PrepareDailyEntries(entries []dsm.Entry) []dsm.DailyEntry {
	rows := dsm.CollapseDailyEntries(entries)
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].TicketURL+"|"+rows[i].Date < rows[j].TicketURL+"|"+rows[j].Date
	})

	previousByURL := map[string]dsm.DailyEntry{}
	for i := range rows {
		prev, ok := previousByURL[rows[i].TicketURL]
		if ok && strings.HasPrefix(github.NormalizeStatus(prev.Status), startStatus) && IsNextWorkday(prev.Date, rows[i].Date) {
			rows[i].ContinuedFromPreviousDay = true
		}
		previousByURL[rows[i].TicketURL] = rows[i]
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Date+"|"+rows[i].TicketURL < rows[j].Date+"|"+rows[j].TicketURL
	})
	return rows
}
