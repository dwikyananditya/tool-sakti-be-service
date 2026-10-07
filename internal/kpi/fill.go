package kpi

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"tool-sakti-be-service/internal/dsm"
	"tool-sakti-be-service/internal/github"
	"tool-sakti-be-service/internal/rekap"
)

// FillWorkbook fills only blank cells in an existing KPI workbook; the original
// styling and row order are left untouched.
func FillWorkbook(data []byte, entries []dsm.Entry, scans map[string]*github.Scan) (*excelize.File, Stats, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, Stats{}, err
	}
	daily := rekap.PrepareDailyEntries(entries)

	sheet := ""
	for _, name := range f.GetSheetList() {
		if regexp.MustCompile(`(?i)^kpi$`).MatchString(name) {
			sheet = name
			break
		}
	}
	if sheet == "" {
		list := f.GetSheetList()
		if len(list) == 0 {
			return nil, Stats{}, fmt.Errorf("workbook has no sheets")
		}
		sheet = list[0]
	}

	matrix, err := f.GetRows(sheet)
	if err != nil {
		return nil, Stats{}, err
	}
	headerIndex := findHeaderRow(matrix)
	if headerIndex < 0 {
		return nil, Stats{}, fmt.Errorf("KPI header row not found")
	}
	headers := normalizeHeaders(matrix[headerIndex])

	if indexOf(headers, "target date") < 0 {
		dateIdx := findColumn(headers, "date", "tanggal")
		insertAt := dateIdx + 1
		colName, _ := excelize.ColumnNumberToName(insertAt + 1)
		if err := f.InsertCols(sheet, colName, 1); err != nil {
			return nil, Stats{}, err
		}
		cell, _ := excelize.CoordinatesToCellName(insertAt+1, headerIndex+1)
		_ = f.SetCellValue(sheet, cell, "Target Date")
		if matrix, err = f.GetRows(sheet); err != nil {
			return nil, Stats{}, err
		}
		headers = normalizeHeaders(matrix[headerIndex])
	}

	cols := struct{ assignee, url, date, targetDate, start, end, hour int }{
		assignee:   findColumn(headers, "assignee", "pic"),
		url:        findColumn(headers, "ticket url", "url"),
		date:       findColumn(headers, "date", "tanggal"),
		targetDate: findColumn(headers, "target date"),
		start:      findColumn(headers, "start time", "start"),
		end:        findColumn(headers, "end time", "end"),
		hour:       findColumn(headers, "hour", "hours", "durasi"),
	}
	if cols.url < 0 || cols.date < 0 || cols.start < 0 || cols.end < 0 {
		return nil, Stats{}, fmt.Errorf("missing Ticket URL, Date, Start Time, or End Time column")
	}

	diagHeaders := []any{"KPI Row", "Assignee", "Date", "Session", "Ticket URL", "Start Time", "End Time", "Hour", "Rule", "Start Source", "Start Source Type", "Start Evidence", "End Source", "End Source Type", "End Evidence", "Confidence", "Needs Review"}
	var diagRows [][]any
	stats := Stats{}

	for rowIndex := headerIndex + 1; rowIndex < len(matrix); rowIndex++ {
		row := matrix[rowIndex]
		url := github.NormalizeURL(cellAt(row, cols.url))
		if url == "" {
			continue
		}
		if cols.targetDate >= 0 {
			setCell(f, sheet, rowIndex, cols.targetDate, displayDateOrBlank(rekap.TargetDateForTicket(url, scans)))
		}
		if cellAt(row, cols.start) != "" || cellAt(row, cols.end) != "" {
			stats.Skipped++
			continue
		}
		date := normalizeDate(cellAt(row, cols.date))
		assignee := NormalizePerson(cellAt(row, cols.assignee))
		var entry *dsm.DailyEntry
		for i := range daily {
			if daily[i].TicketURL == url && daily[i].Date == date && (assignee == "" || NormalizePerson(daily[i].Assignee) == assignee) {
				entry = &daily[i]
				break
			}
		}
		if entry == nil {
			stats.Review++
			continue
		}
		d := rekap.DecideTimes(*entry, scans)
		if !d.HasStart || !d.HasEnd || d.NeedsReview {
			stats.Review++
		}
		startStr, endStr := "", ""
		if d.HasStart {
			startStr = formatDateTime(d.Start)
			setCell(f, sheet, rowIndex, cols.start, startStr)
		}
		if d.HasEnd {
			endStr = formatDateTime(d.End)
			setCell(f, sheet, rowIndex, cols.end, endStr)
		}
		if d.HasHours && cols.hour >= 0 {
			setCellNumber(f, sheet, rowIndex, cols.hour, round2(d.Hours))
		}
		if d.HasStart || d.HasEnd {
			stats.Updated++
		}

		needsReview := "NO"
		if d.NeedsReview {
			needsReview = "YES"
		}
		diagRows = append(diagRows, []any{
			rowIndex + 1, entry.Assignee, entry.Date, entry.Session, entry.TicketURL,
			startStr, endStr, hourCell(d), d.Rule,
			d.StartSource, d.StartSourceKind, d.StartEvidence, d.EndSource, d.EndSourceKind, d.EndEvidence,
			d.Confidence, needsReview,
		})
	}

	for _, name := range []string{"Diagnostic", "Rekap Tiket Unik"} {
		if idx, _ := f.GetSheetIndex(name); idx >= 0 {
			_ = f.DeleteSheet(name)
		}
	}
	f.NewSheet("Diagnostic")
	writeSheet(f, "Diagnostic", diagHeaders, diagRows, sheetStyle{
		headerRGB: "44546A",
		widths:    []float64{8, 16, 12, 10, 55, 20, 20, 10, 30, 48, 18, 55, 48, 18, 55, 12, 14},
		numberCol: map[int]bool{7: true},
	})
	appendUniqueTicketsSheet(f, daily, scans)

	return f, stats, nil
}

func findHeaderRow(matrix [][]string) int {
	for i, row := range matrix {
		norm := normalizeHeaders(row)
		if indexOf(norm, "ticket url") >= 0 && indexOf(norm, "start time") >= 0 {
			return i
		}
	}
	return -1
}

func normalizeHeaders(row []string) []string {
	out := make([]string, len(row))
	for i, v := range row {
		out[i] = multiSpace.ReplaceAllString(strings.TrimSpace(strings.ToLower(v)), " ")
	}
	return out
}

func indexOf(headers []string, name string) int {
	for i, h := range headers {
		if h == name {
			return i
		}
	}
	return -1
}

func findColumn(headers []string, names ...string) int {
	for i, h := range headers {
		for _, n := range names {
			if h == n {
				return i
			}
		}
	}
	return -1
}

func cellAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return row[idx]
}

var (
	ddmmyyyyRe = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})[/-](\d{4})`)
)

func normalizeDate(value string) string {
	text := strings.TrimSpace(value)
	if m := ddmmyyyyRe.FindStringSubmatch(text); m != nil {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		return fmt.Sprintf("%s-%02d-%02d", m[3], month, day)
	}
	if len(text) >= 10 {
		return text[:10]
	}
	return text
}

func setCell(f *excelize.File, sheet string, row, col int, value string) {
	cell, _ := excelize.CoordinatesToCellName(col+1, row+1)
	_ = f.SetCellValue(sheet, cell, value)
}

func setCellNumber(f *excelize.File, sheet string, row, col int, value float64) {
	cell, _ := excelize.CoordinatesToCellName(col+1, row+1)
	_ = f.SetCellValue(sheet, cell, value)
}
