package rekap

import "time"

func parseJakarta(date, clock string) time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", date+"T"+clock, jakarta)
	if err != nil {
		return time.Time{}
	}
	return t
}

// weekday returns the day of week, Sunday=0..Saturday=6, computed at UTC
// midnight so it doesn't shift with the local zone.
func weekday(date string) time.Weekday {
	t, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		return time.Monday
	}
	return t.Weekday()
}

func startOfWorkday(date string) time.Time { return parseJakarta(date, "09:00:00") }

func workdayCloseTime(date string) time.Time {
	wd := weekday(date)
	if wd == time.Sunday {
		return time.Time{}
	}
	if wd == time.Saturday {
		return parseJakarta(date, "16:00:00")
	}
	return parseJakarta(date, "17:00:00")
}

func fallbackStart(date, session string) time.Time {
	if len(session) >= 2 && session[:2] == "11" {
		return parseJakarta(date, "09:00:00")
	}
	return parseJakarta(date, "13:00:00")
}

func EffectiveWorkHours(start, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0
	}
	date := localDate(start)
	if localDate(end) != date {
		return 0
	}
	wd := weekday(date)
	if wd == time.Sunday {
		return 0
	}
	close := "17:00:00"
	if wd == time.Saturday {
		close = "16:00:00"
	}
	breaks := [][2]string{{"12:00:00", "13:00:00"}}
	if wd == time.Friday {
		breaks = [][2]string{{"11:30:00", "13:30:00"}}
	}

	s := maxTime(start, parseJakarta(date, "09:00:00"))
	e := minTime(end, parseJakarta(date, close))
	if !e.After(s) {
		return 0
	}
	dur := e.Sub(s)
	for _, b := range breaks {
		ba := parseJakarta(date, b[0])
		bb := parseJakarta(date, b[1])
		overlap := minTime(e, bb).Sub(maxTime(s, ba))
		if overlap > 0 {
			dur -= overlap
		}
	}
	if dur < 0 {
		return 0
	}
	return dur.Hours()
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func IsNextWorkday(previousDate, currentDate string) bool {
	t, err := time.ParseInLocation("2006-01-02", previousDate, time.UTC)
	if err != nil {
		return false
	}
	t = t.AddDate(0, 0, 1)
	for t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, 1)
	}
	return t.Format("2006-01-02") == currentDate
}
