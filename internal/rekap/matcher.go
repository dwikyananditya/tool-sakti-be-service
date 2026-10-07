// Package rekap turns GitHub timeline data into Start/End work times.
package rekap

import (
	"sort"
	"strings"
	"time"

	"tool-sakti-be-service/internal/dsm"
	"tool-sakti-be-service/internal/github"
)

const startStatus = "in progress"

var jakarta = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}()

type Decision struct {
	Start, End       time.Time
	HasStart, HasEnd bool
	Hours            float64
	HasHours         bool
	Rule             string
	StartSource      string
	EndSource        string
	StartSourceKind  string
	EndSourceKind    string
	StartEvidence    string
	EndEvidence      string
	Confidence       string
	NeedsReview      bool
}

type eventView struct {
	github.Event
	SourceURL  string
	SourceKind string // "parent" | "linked"
}

func localDate(t time.Time) string { return github.JakartaDate(t) }

func relevantEvents(scan *github.Scan, date, kind string) []eventView {
	if scan == nil {
		return nil
	}
	var out []eventView
	for _, e := range scan.Events {
		if localDate(e.DateTime) != date {
			continue
		}
		out = append(out, eventView{Event: e, SourceURL: scan.URL, SourceKind: kind})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DateTime.Before(out[j].DateTime) })
	return out
}

func eventPriority(e eventView, target string) int {
	if e.TargetStatus != "" {
		if e.SourceKind == "parent" && e.TargetStatus == target {
			return 100
		}
		if e.TargetStatus == target {
			return 90
		}
	}
	if target == "deployed" && e.ClosedThis {
		if e.SourceKind == "parent" {
			return 95
		}
		return 85
	}
	if e.WorkActivity {
		if e.SourceKind == "parent" {
			return 80
		}
		return 70
	}
	return 0
}

func pickEnd(events []eventView, target string) *eventView {
	type scored struct {
		ev       eventView
		priority int
	}
	var cands []scored
	for _, e := range events {
		if p := eventPriority(e, target); p > 0 {
			cands = append(cands, scored{e, p})
		}
	}
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].priority != cands[j].priority {
			return cands[i].priority > cands[j].priority
		}
		return cands[i].ev.DateTime.After(cands[j].ev.DateTime)
	})
	ev := cands[0].ev
	return &ev
}

func pickStart(events []eventView, before time.Time) *eventView {
	var candidates []eventView
	for _, e := range events {
		if e.TargetStatus != startStatus {
			continue
		}
		if !before.IsZero() && !e.DateTime.Before(before) {
			continue
		}
		candidates = append(candidates, e)
	}
	for _, e := range candidates {
		if e.SourceKind == "parent" {
			ev := e
			return &ev
		}
	}
	if len(candidates) > 0 {
		ev := candidates[0]
		return &ev
	}
	return nil
}

func DecideTimes(entry dsm.DailyEntry, scans map[string]*github.Scan) Decision {
	parent := scans[entry.TicketURL]
	parentEvents := relevantEvents(parent, entry.Date, "parent")
	var linkedEvents []eventView
	if parent != nil {
		for _, url := range parent.LinkedURLs {
			linkedEvents = append(linkedEvents, relevantEvents(scans[url], entry.Date, "linked")...)
		}
	}
	all := append(append([]eventView{}, parentEvents...), linkedEvents...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].DateTime.Before(all[j].DateTime) })

	target := github.NormalizeStatus(entry.Status)
	sessions := entry.Sessions
	if len(sessions) == 0 && entry.Session != "" {
		sessions = []string{entry.Session}
	}
	firstSession := entry.FirstSession
	if firstSession == "" && len(sessions) > 0 {
		firstSession = sessions[0]
	}
	if firstSession == "" {
		firstSession = entry.Session
	}

	isLateInProgress := strings.HasPrefix(target, startStatus) && anySessionAt16(sessions)
	if isLateInProgress {
		workdayEnd := workdayCloseTime(entry.Date)
		if !workdayEnd.IsZero() {
			startEvent := pickStart(parentEvents, workdayEnd)
			var start time.Time
			startSource := ""
			if entry.ContinuedFromPreviousDay {
				start = startOfWorkday(entry.Date)
				startSource = "CONTINUED 09:00"
			} else if startEvent != nil {
				start = startEvent.DateTime
				startSource = startEvent.SourceURL
			} else {
				start = fallbackStart(entry.Date, firstSession)
				startSource = "DSM " + firstSession
			}
			return decision(start, workdayEnd, "IN_PROGRESS_UNTIL_WORKDAY_END",
				startSource, "WORKDAY_END", "MEDIUM", startEvent, nil)
		}
	}

	end := pickEnd(all, target)
	if end == nil {
		return Decision{Rule: "NEEDS_REVIEW_NO_VALID_END", Confidence: "LOW", NeedsReview: true}
	}

	if strings.HasPrefix(target, startStatus) {
		var later *eventView
		for _, e := range all {
			if e.DateTime.After(end.DateTime) && (e.WorkActivity || e.TargetStatus != "") {
				ev := e
				later = &ev
			}
		}
		if later != nil {
			end = later
		}
	}

	startEvent := pickStart(parentEvents, end.DateTime)
	var start time.Time
	switch {
	case entry.ContinuedFromPreviousDay:
		start = startOfWorkday(entry.Date)
	case startEvent != nil:
		start = startEvent.DateTime
	default:
		start = fallbackStart(entry.Date, firstSession)
	}
	// The start SOURCE label follows the matched event if any, independent of
	// whether the start TIME was overridden by continuation.
	startSource := "DSM " + firstSession
	if startEvent != nil {
		startSource = startEvent.SourceURL
	}

	rule := "DSM_FALLBACK_MATCHED_GITHUB_END"
	confidence := "LOW"
	if startEvent != nil {
		if startEvent.SourceKind == "parent" {
			rule = "PARENT_START_MATCHED_END"
		} else {
			rule = "LINKED_START_MATCHED_END"
		}
		if end.SourceKind == "parent" {
			confidence = "HIGH"
		} else {
			confidence = "MEDIUM"
		}
	}
	return decision(start, end.DateTime, rule, startSource, end.SourceURL, confidence, startEvent, end)
}

func anySessionAt16(sessions []string) bool {
	for _, s := range sessions {
		if strings.HasPrefix(s, "16:") {
			return true
		}
	}
	return false
}

func decision(start, end time.Time, rule, startSource, endSource, confidence string, startEvent, endEvent *eventView) Decision {
	hours := EffectiveWorkHours(start, end)
	d := Decision{
		Start:       start,
		HasStart:    !start.IsZero(),
		End:         end,
		HasEnd:      !end.IsZero(),
		Hours:       hours,
		HasHours:    true,
		Rule:        rule,
		StartSource: startSource,
		EndSource:   endSource,
		Confidence:  confidence,
		NeedsReview: hours <= 0,
	}
	if startEvent != nil {
		d.StartSourceKind = startEvent.SourceKind
		d.StartEvidence = evidence(*startEvent)
	} else if strings.HasPrefix(startSource, "DSM ") {
		d.StartSourceKind = "dsm-fallback"
	}
	if endEvent != nil {
		d.EndSourceKind = endEvent.SourceKind
		d.EndEvidence = evidence(*endEvent)
	}
	return d
}

func evidence(e eventView) string {
	switch {
	case e.TargetStatus != "":
		return "to " + e.TargetStatus
	case e.ClosedThis:
		return "closed this"
	case e.WorkActivity:
		return "work activity"
	default:
		return ""
	}
}
