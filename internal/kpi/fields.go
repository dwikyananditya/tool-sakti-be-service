// Package kpi derives KPI values and renders the output workbook.
package kpi

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var jakarta = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}()

var (
	ticketTypeRe = regexp.MustCompile(`(?i)^(FEAT|FIX|BUGS?|ENHANCE|REFACTOR|CHORE)\s*:`)
	bracketRe    = regexp.MustCompile(`(?i)^\s*\[[^\]]+\]\s*`)
	leadBracket  = regexp.MustCompile(`^\s*\[([^\]]+)\]`)
)

func TicketType(title string) string {
	withoutPrefix := bracketRe.ReplaceAllString(title, "")
	if m := ticketTypeRe.FindStringSubmatch(withoutPrefix); m != nil {
		return strings.ToUpper(m[1])
	}
	return ""
}

func SystemType(title string) string {
	m := leadBracket.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	value := strings.ToUpper(m[1])
	switch {
	case strings.HasPrefix(value, "SUPERAPPS"):
		return "SUPERAPPS"
	case strings.HasPrefix(value, "GOEXPERT"):
		return "GOEXPERT"
	case strings.HasPrefix(value, "RESET_GOA"):
		return "RESET_GOA"
	}
	if i := strings.Index(value, "-"); i >= 0 {
		return value[:i]
	}
	return value
}

func WeekOfMonth(date string) string {
	parts := strings.Split(date, "-")
	if len(parts) != 3 {
		return ""
	}
	year, _ := strconv.Atoi(parts[0])
	month, _ := strconv.Atoi(parts[1])
	day, _ := strconv.Atoi(parts[2])
	if year == 0 || month == 0 || day == 0 {
		return ""
	}
	firstDay := int(time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Weekday())
	mondayOffset := (firstDay + 6) % 7
	return "Minggu " + strconv.Itoa((day+mondayOffset-1)/7+1)
}

var canonical = map[string]string{
	"allif": "allief", "allief": "allief",
	"hizkia": "hizkia", "maulana": "maulana",
	"dwiki": "dwiki", "dwiky": "dwiki",
}

var nonAlpha = regexp.MustCompile(`[^a-z]`)

func NormalizePerson(value string) string {
	cleaned := strings.ToLower(value)
	cleaned = strings.ReplaceAll(cleaned, "gobimbel", "")
	cleaned = nonAlpha.ReplaceAllString(cleaned, "")
	if c, ok := canonical[cleaned]; ok {
		return c
	}
	return cleaned
}

var displayAssigneeMap = map[string]string{
	"allief": "Allief", "hizkia": "Hizkia", "maulana": "Maulana", "dwiki": "Dwiky",
}

func displayAssignee(value string) string {
	if v, ok := displayAssigneeMap[NormalizePerson(value)]; ok {
		return v
	}
	return value
}

var statusDashUnderscore = regexp.MustCompile(`[-_]+`)
var multiSpace = regexp.MustCompile(`\s+`)

func normalizeStatusText(v string) string {
	v = strings.ToLower(v)
	v = statusDashUnderscore.ReplaceAllString(v, " ")
	v = multiSpace.ReplaceAllString(v, " ")
	return strings.TrimSpace(v)
}

var displayStatusMap = map[string]string{
	"todo": "Todo", "in progress": "In Progress", "ready to review": "Ready to Review",
	"staging": "Staging", "deployed": "Deployed",
}

func displayStatus(value string) string {
	if v, ok := displayStatusMap[normalizeStatusText(value)]; ok {
		return v
	}
	return value
}

func displayDate(value string) string {
	parts := strings.Split(value, "-")
	if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
		return parts[2] + "/" + parts[1] + "/" + parts[0]
	}
	return value
}

func displayDateOrBlank(value string) string {
	if value == "" {
		return ""
	}
	return displayDate(value)
}

func formatDateTime(t time.Time) string {
	return t.In(jakarta).Format("02/01/2006 15:04:05")
}

func formatHours(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func sortDateValue(value string) string {
	m := regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{4})$`).FindStringSubmatch(value)
	if m != nil {
		return m[3] + "-" + m[2] + "-" + m[1]
	}
	return value
}

func GeneratedFileName(firstDate string) string {
	parts := strings.Split(firstDate, "-")
	months := map[string]string{
		"01": "Januari", "02": "Februari", "03": "Maret", "04": "April", "05": "Mei", "06": "Juni",
		"07": "Juli", "08": "Agustus", "09": "September", "10": "Oktober", "11": "November", "12": "Desember",
	}
	year, month := "Data", "Export"
	if len(parts) >= 2 {
		year = parts[0]
		if m, ok := months[parts[1]]; ok {
			month = m
		} else {
			month = parts[1]
		}
	}
	return "KPI-" + month + "-" + year + "-JEJAK.xlsx"
}
