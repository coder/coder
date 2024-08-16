package duration

import (
	"fmt"
	"time"
)

type Unit struct {
	value int
	unit  string
}

func Humanize(d time.Duration) string {
	units := []Unit{
		{int(d.Hours() / 24), "day"},
		{int(d.Hours()) % 24, "hour"},
		{int(d.Minutes()) % 60, "minute"},
		{int(d.Seconds()) % 60, "second"},
	}
	nonZeroUnits := []Unit{}
	for _, unit := range units {
		if unit.value > 0 {
			nonZeroUnits = append(nonZeroUnits, unit)
		}
	}
	if len(nonZeroUnits) == 0 {
		return "0 seconds"
	}
	var result string
	for i, unit := range nonZeroUnits {
		if i > 0 {
			if i == len(nonZeroUnits)-1 {
				result += " and "
			} else {
				result += ", "
			}
		}
		result += fmt.Sprintf("%d %s", unit.value, unit.unit)
		if unit.value > 1 {
			result += "s"
		}
	}
	return result
}
