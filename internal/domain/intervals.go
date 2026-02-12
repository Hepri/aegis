package domain

import (
	"sort"
	"strings"
	"time"
)

const (
	// IntervalWindowHours is today + tomorrow
	IntervalWindowHours = 48
)

// TempAccessRange is [Start, Until] for temporary access
type TempAccessRange struct {
	Start time.Time
	End   time.Time
}

// BlockRange is [Start, Until] for block
type BlockRange struct {
	Start time.Time
	End   time.Time
}

// ComputeAllowedIntervals computes allowed intervals for a user
// based on schedule, temporary access requests, and block requests.
// 1) Build from schedule, 2) Add temp access and merge, 3) Cut out each block.
func ComputeAllowedIntervals(
	now time.Time,
	schedule DaySchedule,
	tempAccess []TempAccessRange,
	blocks []BlockRange,
	includePast bool,
) ([]AllowedInterval, time.Time) {
	// Truncate now to minute for stable interval boundaries
	if !includePast {
		now = now.Truncate(time.Minute)
	}

	var intervals []AllowedInterval

	windowEnd := now.Add(IntervalWindowHours * time.Hour)

	// 1. Schedule-based intervals for today and tomorrow
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for dayOffset := 0; dayOffset < 2; dayOffset++ {
		day := today.AddDate(0, 0, dayOffset)
		dayKey := strings.ToLower(day.Weekday().String())
		dayIntervals, ok := schedule[dayKey]
		if !ok {
			continue
		}
		for _, iv := range dayIntervals {
			start, end, err := parseDayInterval(day, iv.Start, iv.End)
			if err != nil {
				continue
			}
			if !includePast && end.Before(now) {
				continue
			}
			if start.After(windowEnd) {
				continue
			}
			if !includePast && start.Before(now) {
				start = now
			}
			if end.After(windowEnd) {
				end = windowEnd
			}
			if end.After(start) {
				intervals = append(intervals, AllowedInterval{Start: start, End: end})
			}
		}
	}

	// 2. Add temporary access intervals
	for _, ta := range tempAccess {
		if !includePast && ta.End.Before(now) {
			continue
		}
		if ta.Start.After(windowEnd) {
			continue
		}
		start, end := ta.Start, ta.End
		if start.Before(now) && !includePast {
			start = now
		}
		if end.After(windowEnd) {
			end = windowEnd
		}
		if end.After(start) {
			intervals = append(intervals, AllowedInterval{Start: start, End: end})
		}
	}

	// 3. Merge overlapping intervals
	intervals = mergeIntervals(intervals)

	// 4. Apply each block: cut out [start, end] from intervals
	for _, b := range blocks {
		if b.End.After(now) && b.Start.Before(b.End) {
			intervals = subtractBlockFromIntervals(intervals, b.Start, b.End)
		}
	}

	// 5. Compute next change time
	nextChange := windowEnd
	for _, iv := range intervals {
		if iv.Start.After(now) && iv.Start.Before(nextChange) {
			nextChange = iv.Start
		}
		if iv.End.After(now) && iv.End.Before(nextChange) {
			nextChange = iv.End
		}
	}
	for _, ta := range tempAccess {
		if ta.End.After(now) && ta.End.Before(nextChange) {
			nextChange = ta.End
		}
	}
	for _, b := range blocks {
		if b.Start.After(now) && b.Start.Before(nextChange) {
			nextChange = b.Start
		}
		if b.End.After(now) && b.End.Before(nextChange) {
			nextChange = b.End
		}
	}

	return intervals, nextChange
}

// subtractBlockFromIntervals cuts [blockStart, blockUntil] out of each interval.
// An interval may be split into two parts (before and after the block).
func subtractBlockFromIntervals(intervals []AllowedInterval, blockStart, blockUntil time.Time) []AllowedInterval {
	var result []AllowedInterval
	for _, iv := range intervals {
		// No overlap: interval entirely before block or entirely after
		if !iv.End.After(blockStart) || !iv.Start.Before(blockUntil) {
			result = append(result, iv)
			continue
		}
		// Overlap: keep parts before blockStart and after blockUntil
		if iv.Start.Before(blockStart) {
			result = append(result, AllowedInterval{Start: iv.Start, End: blockStart})
		}
		if iv.End.After(blockUntil) {
			result = append(result, AllowedInterval{Start: blockUntil, End: iv.End})
		}
	}
	return result
}

func parseDayInterval(day time.Time, startStr, endStr string) (time.Time, time.Time, error) {
	sh, sm, err := ParseTime(startStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	eh, em, err := ParseTime(endStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), sh, sm, 0, 0, day.Location())
	end := time.Date(day.Year(), day.Month(), day.Day(), eh, em, 0, 0, day.Location())
	if !end.After(start) {
		end = end.Add(24 * time.Hour)
	}
	return start, end, nil
}

func mergeIntervals(intervals []AllowedInterval) []AllowedInterval {
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].Start.Before(intervals[j].Start)
	})
	merged := []AllowedInterval{intervals[0]}
	for i := 1; i < len(intervals); i++ {
		last := &merged[len(merged)-1]
		if intervals[i].Start.Before(last.End) || intervals[i].Start.Equal(last.End) {
			if intervals[i].End.After(last.End) {
				last.End = intervals[i].End
			}
		} else {
			merged = append(merged, intervals[i])
		}
	}
	return merged
}
