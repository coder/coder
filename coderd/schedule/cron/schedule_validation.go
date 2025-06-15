package cron

import (
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// ValidateSchedules checks if any schedules overlap
func ValidateSchedules(schedules []*Schedule) error {
	for i := 0; i < len(schedules); i++ {
		for j := i + 1; j < len(schedules); j++ {
			overlap, err := SchedulesOverlap(schedules[i], schedules[j])
			if err != nil {
				return xerrors.Errorf("invalid schedule: %w", err)
			}
			if overlap {
				return xerrors.Errorf("schedules overlap: %s and %s",
					schedules[i].Cron(), schedules[j].Cron())
			}
		}
	}
	return nil
}

// SchedulesOverlap checks if two schedules overlap by checking
// days, months, and hours separately
func SchedulesOverlap(s1, s2 *Schedule) (bool, error) {
	// Get cron fields
	fields1 := strings.Fields(s1.Cron())
	fields2 := strings.Fields(s2.Cron())

	// Check if months overlap
	monthsOverlap, err := MonthsOverlap(fields1[3], fields2[3])
	if err != nil {
		return false, xerrors.Errorf("invalid month range: %w", err)
	}
	if !monthsOverlap {
		return false, nil
	}

	// Check if days overlap (DOM OR DOW)
	daysOverlap, err := DaysOverlap(fields1[2], fields1[4], fields2[2], fields2[4])
	if err != nil {
		return false, xerrors.Errorf("invalid day range: %w", err)
	}
	if !daysOverlap {
		return false, nil
	}

	// Check if hours overlap
	hoursOverlap, err := HoursOverlap(fields1[1], fields2[1])
	if err != nil {
		return false, xerrors.Errorf("invalid hour range: %w", err)
	}

	return hoursOverlap, nil
}

// MonthsOverlap checks if two month ranges overlap
func MonthsOverlap(months1, months2 string) (bool, error) {
	return CheckOverlap(months1, months2, 12)
}

// HoursOverlap checks if two hour ranges overlap
func HoursOverlap(hours1, hours2 string) (bool, error) {
	return CheckOverlap(hours1, hours2, 23)
}

// DomOverlap checks if two day-of-month ranges overlap
func DomOverlap(dom1, dom2 string) (bool, error) {
	return CheckOverlap(dom1, dom2, 31)
}

// DowOverlap checks if two day-of-week ranges overlap
func DowOverlap(dow1, dow2 string) (bool, error) {
	return CheckOverlap(dow1, dow2, 6)
}

// DaysOverlap checks if two day ranges overlap, considering both DOM and DOW.
// Returns true if either DOM ranges overlap OR DOW ranges overlap.
func DaysOverlap(dom1, dow1, dom2, dow2 string) (bool, error) {
	// Check if either DOM or DOW overlaps
	domOverlap, err := DomOverlap(dom1, dom2)
	if err != nil {
		return false, err
	}
	dowOverlap, err := DowOverlap(dow1, dow2)
	if err != nil {
		return false, err
	}

	return domOverlap || dowOverlap, nil
}

// CheckOverlap is a generic function to check if two ranges overlap
func CheckOverlap(range1, range2 string, maxValue int) (bool, error) {
	set1, err := ParseRange(range1, maxValue)
	if err != nil {
		return false, err
	}
	set2, err := ParseRange(range2, maxValue)
	if err != nil {
		return false, err
	}

	for value := range set1 {
		if set2[value] {
			return true, nil
		}
	}
	return false, nil
}

// ParseRange converts a cron range to a set of integers
// maxValue is the maximum allowed value (e.g., 23 for hours, 6 for DOW, 12 for months, 31 for DOM)
func ParseRange(input string, maxValue int) (map[int]bool, error) {
	result := make(map[int]bool)

	// Handle "*" case
	if input == "*" {
		for i := 0; i <= maxValue; i++ {
			result[i] = true
		}
		return result, nil
	}

	// Parse ranges like "1-3,5,7-9"
	parts := strings.Split(input, ",")
	for _, part := range parts {
		if strings.Contains(part, "-") {
			// Handle range like "1-3"
			rangeParts := strings.Split(part, "-")
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, xerrors.Errorf("invalid start value in range: %w", err)
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, xerrors.Errorf("invalid end value in range: %w", err)
			}

			// Validate range
			if start < 0 || end > maxValue || start > end {
				return nil, xerrors.Errorf("invalid range %d-%d: values must be between 0 and %d", start, end, maxValue)
			}

			for i := start; i <= end; i++ {
				result[i] = true
			}
		} else {
			// Handle single value
			value, err := strconv.Atoi(part)
			if err != nil {
				return nil, xerrors.Errorf("invalid value: %w", err)
			}

			// Validate value
			if value < 0 || value > maxValue {
				return nil, xerrors.Errorf("invalid value %d: must be between 0 and %d", value, maxValue)
			}

			result[value] = true
		}
	}
	return result, nil
}
