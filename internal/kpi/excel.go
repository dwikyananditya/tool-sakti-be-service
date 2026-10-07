package kpi

import (
	"math"
	"sort"

	"github.com/xuri/excelize/v2"

	"tool-sakti-be-service/internal/dsm"
	"tool-sakti-be-service/internal/github"
	"tool-sakti-be-service/internal/rekap"
)

type Stats struct {
	Updated int
	Skipped int
	Review  int
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func hourCell(d rekap.Decision) any {
	if !d.HasHours {
		return ""
	}
	return round2(d.Hours)
}

func BuildFromDSM(entries []dsm.Entry, scans map[string]*github.Scan) (*excelize.File, Stats, error) {
	daily := rekap.PrepareDailyEntries(entries)

	kpiHeaders := []any{"Assignee", "Type", "Ticket Title", "Ticket URL", "Type", "Status", "Priority", "Date", "Target Date", "Week", "Start Time", "End Time", "Hour"}
	diagHeaders := []any{"KPI Row", "Assignee", "Date", "DSM Sessions", "Occurrences", "Ticket URL", "Start Time", "End Time", "Hour", "Rule", "Start Source", "Start Source Type", "Start Evidence", "End Source", "End Source Type", "End Evidence", "Confidence", "Needs Review"}

	var kpiRows [][]any
	var diagRows [][]any
	stats := Stats{}

	for i, entry := range daily {
		d := rekap.DecideTimes(entry, scans)
		if !d.HasStart || !d.HasEnd || d.NeedsReview {
			stats.Review++
		}
		start, end := "", ""
		if d.HasStart {
			start = formatDateTime(d.Start)
		}
		if d.HasEnd {
			end = formatDateTime(d.End)
		}
		kpiRows = append(kpiRows, []any{
			displayAssignee(entry.Assignee), SystemType(entry.TicketTitle), entry.TicketTitle,
			entry.TicketURL, TicketType(entry.TicketTitle), entry.Status, "", displayDate(entry.Date),
			displayDateOrBlank(rekap.TargetDateForTicket(entry.TicketURL, scans)), WeekOfMonth(entry.Date),
			start, end, hourCell(d),
		})
		needsReview := "NO"
		if d.NeedsReview || !d.HasEnd {
			needsReview = "YES"
		}
		diagRows = append(diagRows, []any{
			i + 2, displayAssignee(entry.Assignee), entry.Date, joinSessions(entry.Sessions), max(entry.Occurrences, 1),
			entry.TicketURL, start, end, hourCell(d), d.Rule,
			d.StartSource, d.StartSourceKind, d.StartEvidence, d.EndSource, d.EndSourceKind, d.EndEvidence,
			d.Confidence, needsReview,
		})
	}
	stats.Updated = len(kpiRows)

	sort.SliceStable(kpiRows, func(a, b int) bool {
		ka := sortDateValue(kpiRows[a][7].(string)) + "|" + kpiRows[a][3].(string)
		kb := sortDateValue(kpiRows[b][7].(string)) + "|" + kpiRows[b][3].(string)
		return ka < kb
	})

	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "KPI")
	writeSheet(f, "KPI", kpiHeaders, kpiRows, sheetStyle{
		headerRGB: "1F4E78",
		widths:    []float64{14, 14, 58, 58, 14, 20, 12, 14, 14, 12, 22, 22, 12},
		numberCol: map[int]bool{12: true},
		zebra:     true,
	})

	f.NewSheet("Diagnostic")
	writeSheet(f, "Diagnostic", diagHeaders, diagRows, sheetStyle{
		headerRGB: "44546A",
		widths:    []float64{9, 14, 13, 20, 12, 58, 22, 22, 10, 30, 48, 18, 55, 48, 18, 55, 12, 14},
		numberCol: map[int]bool{8: true},
	})

	appendUniqueTicketsSheet(f, daily, scans)
	return f, stats, nil
}

func appendUniqueTicketsSheet(f *excelize.File, daily []dsm.DailyEntry, scans map[string]*github.Scan) {
	order := []string{}
	groups := map[string][]dsm.DailyEntry{}
	for _, e := range daily {
		key := NormalizePerson(e.Assignee) + "|" + e.TicketURL
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], e)
	}

	headers := []any{"Assignee", "Type", "Ticket Title", "Ticket URL", "Status", "Priority", "Date", "End Date", "Week"}
	var rows [][]any
	for _, key := range order {
		group := groups[key]
		ordered := append([]dsm.DailyEntry{}, group...)
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Date < ordered[j].Date })
		first, latest := ordered[0], ordered[len(ordered)-1]
		period := rekap.UniqueTicketPeriod(ordered, scans)

		var starts []string
		for _, e := range ordered {
			if d := rekap.DecideTimes(e, scans); d.HasStart {
				starts = append(starts, github.JakartaDate(d.Start))
			}
		}
		sort.Strings(starts)
		exactStartDate := period.StartDate
		if len(starts) > 0 {
			exactStartDate = starts[0]
		}

		status := rekap.LatestGitHubStatus(first.TicketURL, scans)
		if status == "" {
			status = latest.Status
		}
		endDate := ""
		if period.EndDate != "" {
			endDate = displayDate(period.EndDate)
		}
		rows = append(rows, []any{
			displayAssignee(first.Assignee), SystemType(first.TicketTitle), first.TicketTitle, first.TicketURL,
			displayStatus(status), "", displayDate(exactStartDate), endDate, WeekOfMonth(exactStartDate),
		})
	}

	sort.SliceStable(rows, func(a, b int) bool {
		ka := sortDateValue(rows[a][6].(string)) + "|" + rows[a][0].(string) + "|" + rows[a][3].(string)
		kb := sortDateValue(rows[b][6].(string)) + "|" + rows[b][0].(string) + "|" + rows[b][3].(string)
		return ka < kb
	})

	f.NewSheet("Rekap Tiket Unik")
	writeSheet(f, "Rekap Tiket Unik", headers, rows, sheetStyle{
		headerRGB: "548235",
		widths:    []float64{14, 14, 58, 58, 20, 12, 14, 14, 12},
	})
}

func joinSessions(sessions []string) string {
	out := ""
	for i, s := range sessions {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
