package kpi

import (
	"sort"

	"tool-sakti-be-service/internal/dsm"
	"tool-sakti-be-service/internal/github"
	"tool-sakti-be-service/internal/rekap"
)

type PreviewRow struct {
	Assignee    string `json:"assignee"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Date        string `json:"date"`
	Status      string `json:"status"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Hours       string `json:"hours"`
	Confidence  string `json:"confidence"`
	Rule        string `json:"rule"`
	NeedsReview bool   `json:"needsReview"`
}

func PreviewRows(entries []dsm.Entry, scans map[string]*github.Scan) ([]PreviewRow, Stats) {
	daily := rekap.PrepareDailyEntries(entries)
	rows := make([]PreviewRow, 0, len(daily))
	stats := Stats{}

	for _, entry := range daily {
		d := rekap.DecideTimes(entry, scans)
		if !d.HasStart || !d.HasEnd || d.NeedsReview {
			stats.Review++
		}
		row := PreviewRow{
			Assignee:    displayAssignee(entry.Assignee),
			Title:       entry.TicketTitle,
			URL:         entry.TicketURL,
			Date:        displayDate(entry.Date),
			Status:      entry.Status,
			Confidence:  d.Confidence,
			Rule:        d.Rule,
			NeedsReview: d.NeedsReview || !d.HasEnd,
		}
		if d.HasStart {
			row.Start = formatDateTime(d.Start)
		}
		if d.HasEnd {
			row.End = formatDateTime(d.End)
		}
		if d.HasHours {
			row.Hours = formatHours(round2(d.Hours))
		}
		rows = append(rows, row)
	}
	stats.Updated = len(rows)

	sort.SliceStable(rows, func(a, b int) bool {
		return sortDateValue(rows[a].Date)+"|"+rows[a].URL < sortDateValue(rows[b].Date)+"|"+rows[b].URL
	})
	return rows, stats
}
