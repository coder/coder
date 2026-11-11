package cron

import (
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
// @daily are not supported. timeZone is an IANA time zone name other than
// "Local". A spec that never runs, such as "0 0 30 2 *", is an error.
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

	switch timeZone {
	case "":
		return nil, xerrors.New("time zone is empty")
	case "Local":
		return nil, xerrors.New(`time zone "Local" is not supported, use an IANA time zone name`)
	}
	if _, err := time.LoadLocation(timeZone); err != nil {
		return nil, xerrors.Errorf("load time zone %q: %w", timeZone, err)
	}

	sched, err := parse("CRON_TZ=" + timeZone + " " + strings.Join(fields, " "))
	if err != nil {
		return nil, err
	}
	sched.wallClock = true

	// Min would never return for a schedule without a run.
	if naive := sched.naive(); naive.Next(intervalAnchor).IsZero() {
		return nil, xerrors.Errorf("schedule %q never runs", strings.Join(fields, " "))
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

// nextWallClock implements Next for schedules from Standard. It walks the
// matching wall-clock readings in order and returns the first one whose
// instant is after t. A reading that repeats resolves to the instant of its
// first occurrence, so its second occurrence is not after t once the first
// has passed. Readings that do not exist resolve to the same instant, so they
// run once.
func (s Schedule) nextWallClock(t time.Time) time.Time {
	loc := s.sched.Location
	naive := s.naive()
	for wall := nextReading(&naive, wallClock(t, loc)); !wall.IsZero(); wall = nextReading(&naive, wall) {
		if instant := resolveWallClock(wall, loc); instant.After(t) {
			return instant.In(t.Location())
		}
	}
	return time.Time{}
}

// nextReading returns the first reading after w that naive matches, or the
// zero time when there is none. robfig/cron gives up after five years, but two
// leap days are eight years apart across 2100, so it searches again from the
// years it did not cover. Three searches reach 18 years, past any gap.
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

// resolveWallClock returns the instant at which the wall clock in loc shows
// wall, a reading held as a UTC time. For a reading that occurs twice it
// returns the first occurrence. For a reading that does not exist it returns
// the start of the zone period after the gap. It never decreases as wall
// increases.
func resolveWallClock(wall time.Time, loc *time.Location) time.Time {
	// A zone offset is within 26 hours of UTC (RFC 8536 allows up to 25:59:59,
	// and old data such as America/Metlakatla in 1867 goes past 15 hours), so
	// no instant that shows wall is earlier than this. Walk the zone periods
	// from here.
	for instant := wall.Add(-26 * time.Hour); ; {
		local := instant.In(loc)
		_, offset := local.Zone()
		shift := time.Duration(offset) * time.Second
		start, end := local.ZoneBounds()

		switch {
		case !end.IsZero() && !wall.Before(end.Add(shift)):
			// The period ends before the clock reaches wall.
			instant = end
		case !start.IsZero() && wall.Before(start.Add(shift)):
			// The clock skipped wall when this period started.
			return start
		default:
			return wall.Add(-shift)
		}
	}
}

// intervalAnchor starts the scan of MinInterval. The 28 years from it hold
// every calendar a year can have (leap or not, starting on each weekday), so
// they hold the shortest gap of every year.
var intervalAnchor = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// MinInterval returns the shortest time between two consecutive runs of the
// schedule, measured on its wall clock. It looks at 28 years of runs from
// 2000-01-01, which is enough to see the shortest gap in any year. It returns
// the length of that period when fewer than two runs fall inside it.
//
// Daylight saving changes do not count. Clocks going forward can make one gap
// shorter by up to the size of the change: runs at 01:30 and 03:30 are two
// hours apart on the wall clock and one hour apart on that day.
//
// Unlike Min, which only looks at one week from 1970-01-01, MinInterval sees
// close runs on days that week does not contain.
func (s Schedule) MinInterval() time.Duration {
	// Every matching day runs at the same times of day.
	var times []time.Duration
	for hour := range 24 {
		if s.sched.Hour&(1<<hour) == 0 {
			continue
		}
		for minute := range 60 {
			if s.sched.Minute&(1<<minute) != 0 {
				times = append(times, time.Duration(hour)*time.Hour+time.Duration(minute)*time.Minute)
			}
		}
	}

	end := intervalAnchor.AddDate(28, 0, 0)
	shortest := end.Sub(intervalAnchor)
	for i := 1; i < len(times); i++ {
		shortest = min(shortest, times[i]-times[i-1])
	}

	// Gaps from the last run of a matching day to the first run of the next.
	// The shortest one is between two days in a row, so stop once it shows.
	naive := s.naive()
	lastOfDay := times[len(times)-1]
	nextDay := 24*time.Hour + times[0] - lastOfDay
	for day := naive.Next(intervalAnchor); !day.IsZero() && !day.After(end); {
		midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		last := midnight.Add(lastOfDay)
		// The first run after midnight is the first run of the next matching day.
		next := naive.Next(midnight.AddDate(0, 0, 1).Add(-time.Second))
		if next.IsZero() || next.After(end) {
			break
		}
		gap := next.Sub(last)
		shortest = min(shortest, gap)
		if gap == nextDay {
			break
		}
		day = next
	}
	return shortest
}
