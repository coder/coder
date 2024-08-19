package duration

import (
	"time"

	"github.com/dustin/go-humanize"
)

func Humanize(d time.Duration) string {
	endTime := time.Now().Add(d)
	return humanize.Time(endTime)
}
