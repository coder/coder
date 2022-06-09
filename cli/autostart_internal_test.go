package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:paralleltest // t.Setenv
func TestParseCLISchedule(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		input            []string
		expectedSchedule string
		expectedError    string
		tzEnv            string
	}{
		{
			name:             "DefaultSchedule",
			input:            []string{"Sun-Sat", "09:00AM", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "UTC",
		},
		{
			name:             "DefaultSchedule24Hour",
			input:            []string{"Sun-Sat", "09:00", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "UTC",
		},
		{
			name:             "TimeOfDayOnly",
			input:            []string{"09:00AM"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "America/Chicago",
		},
		{
			name:             "DayOfWeekAndTime",
			input:            []string{"Sun-Sat", "09:00AM"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * Sun-Sat",
			tzEnv:            "America/Chicago",
		},
		{
			name:             "TimeAndLocation",
			input:            []string{"09:00AM", "America/Chicago"},
			expectedSchedule: "CRON_TZ=America/Chicago 0 9 * * *",
			tzEnv:            "UTC",
		},
		{
			name:          "InvalidTime",
			input:         []string{"9am"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "DayOfWeekAndInvalidTime",
			input:         []string{"Sun-Sat", "9am"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "InvalidTimeAndLocation",
			input:         []string{"9:", "America/Chicago"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "DayOfWeekAndInvalidTimeAndLocation",
			input:         []string{"Sun-Sat", "9am", "America/Chicago"},
			expectedError: errInvalidTimeFormat.Error(),
		},
		{
			name:          "WhoKnows",
			input:         []string{"Time", "is", "a", "human", "construct"},
			expectedError: errInvalidScheduleFormat.Error(),
		},
	} {
		testCase := testCase
		//nolint:paralleltest // t.Setenv
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("TZ", testCase.tzEnv)
			actualSchedule, actualError := parseCLISchedule(testCase.input...)
			if testCase.expectedError != "" {
				assert.Nil(t, actualSchedule)
				assert.ErrorContains(t, actualError, testCase.expectedError)
				return
			}
			assert.NoError(t, actualError)
			if assert.NotEmpty(t, actualSchedule) {
				assert.Equal(t, testCase.expectedSchedule, actualSchedule.String())
			}
		})
	}
}
