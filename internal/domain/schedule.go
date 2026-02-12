package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TimeInterval represents allowed time within a day (e.g. 09:00-11:00)
type TimeInterval struct {
	Start string `json:"start"` // "09:00" HH:MM
	End   string `json:"end"`   // "11:00" HH:MM
}

// DaySchedule is a map: day name -> list of intervals
type DaySchedule map[string][]TimeInterval

// ParseTime parses "HH:MM" or "H:MM" format
func ParseTime(s string) (hour, minute int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format: %s", s)
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid time: %s", s)
	}
	return hour, minute, nil
}

// IsWithinInterval checks if given time is within interval (time is same day)
func IsWithinInterval(t time.Time, start, end string) (bool, error) {
	sh, sm, err := ParseTime(start)
	if err != nil {
		return false, err
	}
	eh, em, err := ParseTime(end)
	if err != nil {
		return false, err
	}
	startMins := sh*60 + sm
	endMins := eh*60 + em
	nowMins := t.Hour()*60 + t.Minute()
	return nowMins >= startMins && nowMins < endMins, nil
}
