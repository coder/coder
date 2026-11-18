package cron

import (
	"slices"
	"strings"
	"time"

	rbcron "github.com/robfig/cron/v3"
	"golang.org/x/xerrors"
)

// Standard parses a Schedule from a standard five-field cron spec (minute,
// hour, day of month, month, day of week) that runs on the wall clock of
// timeZone. Unlike Weekly and Daily, every field can be restricted. When both
// day of month and day of week are restricted, a day matches if either field
// matches.
//
// The spec must not start with CRON_TZ= or TZ=, and descriptors such as
// @daily are not supported. timeZone is an IANA time zone name such as
// "America/New_York", and Standard accepts it only when no segment is empty,
// "." or "..", the first segment is not "posix" or "right" (directories of zone
// data that hold other versions of the zones), the name is not "Local" (a Go
// name for the zone of the host), its last segment is not "localtime",
// "posixrules" or "Factory" (files of zone data that are not zones), and
// time.LoadLocation loads it on this host. A spec that never runs, such as
// "0 0 30 2 *", is an error.
//
// The schedule follows local time across daylight saving changes. A repeated
// local time runs at its first instance only. A nonexistent local time runs at
// the first local time after the change, and all nonexistent times of one
// change, together with a time scheduled exactly at the end of the change, run
// once.
//
// Example Usage:
//
//	sched, _ := cron.Standard("0 9 1 * *", "Europe/Berlin")
//	fmt.Println(sched.Next(time.Date(2022, 4, 15, 0, 0, 0, 0, time.UTC)).Format(time.RFC3339))
//	// Output: 2022-05-01T07:00:00Z
func Standard(spec, timeZone string) (*Schedule, error) {
	fields := strings.Fields(spec)
	switch {
	case len(fields) == 0:
		return nil, xerrors.New("cron expression is empty")
	case strings.HasPrefix(fields[0], "CRON_TZ=") || strings.HasPrefix(fields[0], "TZ="):
		return nil, xerrors.New("cron expression must not start with CRON_TZ= or TZ=: pass the time zone as its own argument")
	case len(fields) != 5:
		return nil, xerrors.Errorf("expected 5 fields, got %d (minute, hour, day of month, month, day of week)", len(fields))
	}

	switch {
	case timeZone == "":
		return nil, xerrors.New("time zone is empty")
	case !isZoneName(timeZone):
		return nil, xerrors.Errorf("time zone %q is not a zone name, use an IANA time zone name such as America/New_York", timeZone)
	case isNotZone(timeZone):
		return nil, xerrors.Errorf("time zone %q is not supported, use an IANA time zone name", timeZone)
	}
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		// Go's error holds the name too, so it is left out.
		return nil, xerrors.Errorf("unknown time zone %q", timeZone)
	}
	return standardIn(fields, loc)
}

// otherVersions are directories of zone data that hold other versions of the
// zones: "right/" counts leap seconds, which gives wrong runs, and "posix/"
// repeats the zones of the top directory.
var otherVersions = []string{"posix", "right"}

// notZoneFiles are files of zone data that are not zones. What they give
// depends on the host.
var notZoneFiles = []string{"localtime", "posixrules", "Factory"}

// isZoneName reports whether name has no empty, "." or ".." segment, which also
// means that it does not start or end with a slash, and does not start in a
// directory of other versions. time.LoadLocation cleans the path, so
// "./localtime" reaches the file that "localtime" names, and "America//New_York"
// a zone under another name.
func isZoneName(name string) bool {
	segments := strings.Split(name, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return !slices.Contains(otherVersions, segments[0])
}

// isNotZone reports whether name is "Local", a Go name for the zone of the host,
// or ends in a file of zone data that is not a zone.
func isNotZone(name string) bool {
	segments := strings.Split(name, "/")
	return name == "Local" || slices.Contains(notZoneFiles, segments[len(segments)-1])
}

// standardIn is Standard for fields that have passed its checks and a zone
// that the caller loaded.
func standardIn(fields []string, loc *time.Location) (*Schedule, error) {
	cronStr := strings.Join(fields, " ")
	parsed, err := defaultParser.Parse(cronStr)
	if err != nil {
		return nil, xerrors.Errorf("parse schedule: %w", err)
	}
	spec, ok := parsed.(*rbcron.SpecSchedule)
	if !ok {
		return nil, xerrors.Errorf("expected *cron.SpecSchedule but got %T", parsed)
	}
	spec.Location = loc
	sched := &Schedule{sched: spec, cronStr: cronStr, wallClock: true}

	// Any start works, because nextReading searches past the longest gap
	// between runs of a spec that runs at all.
	naive := sched.naive()
	if nextReading(&naive, time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)).IsZero() {
		return nil, xerrors.Errorf("schedule %q never runs", cronStr)
	}
	return sched, nil
}

// naive returns the schedule evaluated in UTC, which has no daylight saving
// changes. Its times are wall-clock readings, not instants.
func (s Schedule) naive() rbcron.SpecSchedule {
	naive := *s.sched
	naive.Location = time.UTC
	return naive
}

// maxReadings bounds the walk in nextWallClock. A reading more than 28 hours
// after t resolves to an instant after t, and the walk starts at most 26 hours
// before t, so at most 54 hours of readings come first: 3,240 for a schedule
// that matches every minute.
const maxReadings = 1 << 13

// nextWallClock implements Next for schedules from Standard. It walks the
// matching wall-clock readings in order and returns the first one whose
// instant is after t. A reading that repeats resolves to the instant of its
// first occurrence, so its second occurrence is not after t once the first
// has passed. Readings that do not exist resolve to the same instant, so they
// run once. It returns the zero time when a step does not move forward or the
// walk reaches maxReadings, which real zone data cannot cause.
func (s Schedule) nextWallClock(t time.Time) time.Time {
	loc := s.sched.Location
	naive := s.naive()
	wall := wallClock(t, loc)
	for range maxReadings {
		next := nextReading(&naive, wall)
		if next.IsZero() || !next.After(wall) {
			return time.Time{}
		}
		wall = next
		if instant := resolveWallClock(wall, loc); instant.After(t) {
			return instant.In(t.Location())
		}
	}
	return time.Time{}
}

// nextReading returns the first reading after w that naive matches, or the
// zero time when there is none. robfig/cron gives up after the fifth year after
// the year of its start, but two leap days are eight years apart across 2100, so
// it searches again from the years it did not cover. Three searches reach the
// end of the sixteenth year after the year of w, at least 16 years, past any gap.
func nextReading(naive *rbcron.SpecSchedule, w time.Time) time.Time {
	for range 3 {
		if next := naive.Next(w); !next.IsZero() {
			return next
		}
		w = time.Date(w.Year()+6, time.January, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)
	}
	return time.Time{}
}

// wallClock returns the reading of a wall clock in loc at instant t, as a UTC
// time.
func wallClock(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	year, month, day := local.Date()
	hour, minute, second := local.Clock()
	return time.Date(year, month, day, hour, minute, second, local.Nanosecond(), time.UTC)
}

// A UTC offset is at most 25:59:59 either way (RFC 8536), so the instant at
// which a clock shows a reading is within 26 hours of the reading read as UTC.
// Zone periods last days, so one probe every four hours sees every offset in
// effect in that range, with a margin at both ends.
const (
	probeSpan = 28 * time.Hour
	probeStep = 4 * time.Hour
)

// resolveWallClock returns the instant at which the wall clock in loc shows
// wall, a reading held as a UTC time. For a reading that occurs twice it
// returns the first occurrence. For a reading that does not exist it returns
// the instant of the clock change. It never decreases as wall increases. It
// works from the offsets of the zone, never from its period bounds, which Go
// reports wrongly after the last transition in the zone data. Every loop in it
// has a fixed bound.
func resolveWallClock(wall time.Time, loc *time.Location) time.Time {
	// Each offset in effect gives one instant at which the clock could show
	// wall. It does when the offset is still in effect at that instant.
	var first, before, after time.Time
	for probe := -probeSpan; probe <= probeSpan; probe += probeStep {
		_, offset := wall.Add(probe).In(loc).Zone()
		instant := wall.Add(-time.Duration(offset) * time.Second)
		switch shown := wallClock(instant, loc); {
		case shown.Equal(wall):
			if first.IsZero() || instant.Before(first) {
				first = instant
			}
		case shown.Before(wall):
			if before.IsZero() || instant.After(before) {
				before = instant
			}
		default:
			if after.IsZero() || instant.Before(after) {
				after = instant
			}
		}
	}
	if !first.IsZero() {
		return first
	}

	// The clock skipped wall. At before it shows less, at after it shows more,
	// so the change is between them. Find the first second that shows wall or
	// more.
	if before.IsZero() || after.IsZero() || !before.Before(after) {
		// Real zone data cannot get here.
		_, offset := wall.In(loc).Zone()
		return wall.Add(-time.Duration(offset) * time.Second)
	}
	for range 64 {
		mid := before.Add(after.Sub(before) / 2).Truncate(time.Second)
		if !mid.After(before) {
			break
		}
		if wallClock(mid, loc).Before(wall) {
			before = mid
		} else {
			after = mid
		}
	}
	return after
}

// MinInterval returns the shortest distance between two times of day at which
// the schedule runs, counted around the clock: the distance from the last time
// of day to the first time of the next day counts too. A schedule with one time
// of day gives 24 hours. It reads only the hour and minute fields, so it needs
// no time zone and no calendar.
//
// While the UTC offset of the zone does not change, no two runs are closer than
// this value, whatever the day of month, month and day of week fields say. It
// has two limits:
//
//   - A change of the UTC offset (daylight saving) can shorten a gap that
//     spans it, by up to the size of the change. "1,58 1,3 * * *" in
//     America/New_York gives 57 minutes, and on 2026-03-08 its runs at 01:58 and
//     03:01 are 3 minutes apart.
//   - A pair of times across midnight counts even when the day fields never
//     match two days in a row, at any length of interval, also below 24 hours.
//     "0,58 0,23 * * 1" gives 2 minutes, and its runs are 58 minutes apart at
//     least. A check that rejects a schedule when MinInterval is below a
//     minimum can reject a schedule that the minimum would allow.
//
// Above 24 hours it says nothing: a schedule that runs once a week gives 24
// hours.
func (s Schedule) MinInterval() time.Duration {
	var first, last time.Duration
	shortest := 24 * time.Hour
	seen := false
	for hour := range 24 {
		if s.sched.Hour&(1<<hour) == 0 {
			continue
		}
		for minute := range 60 {
			if s.sched.Minute&(1<<minute) == 0 {
				continue
			}
			timeOfDay := time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute
			if !seen {
				first, seen = timeOfDay, true
			} else {
				shortest = min(shortest, timeOfDay-last)
			}
			last = timeOfDay
		}
	}
	return min(shortest, 24*time.Hour+first-last)
}
