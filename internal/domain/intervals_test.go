package domain

import (
	"testing"
	"time"
)

func TestComputeAllowedIntervals_ScheduleOnly(t *testing.T) {
	loc := time.UTC
	// Thursday 12 Feb 2026, 10:00
	now := time.Date(2026, 2, 12, 10, 0, 0, 0, loc)
	schedule := DaySchedule{
		"thursday": {{Start: "07:00", End: "13:15"}},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, nil, nil, false)
	if len(intervals) != 1 {
		t.Fatalf("want 1 interval, got %d", len(intervals))
	}
	if !intervals[0].Start.Equal(now) {
		t.Errorf("start: want %v (clipped to now), got %v", now, intervals[0].Start)
	}
	expectEnd := time.Date(2026, 2, 12, 13, 15, 0, 0, loc)
	if !intervals[0].End.Equal(expectEnd) {
		t.Errorf("end: want %v, got %v", expectEnd, intervals[0].End)
	}
}

func TestComputeAllowedIntervals_TempAccessOutsideSchedule(t *testing.T) {
	loc := time.UTC
	// Thursday 12 Feb 2026, 17:00 - outside schedule
	now := time.Date(2026, 2, 12, 17, 0, 0, 0, loc)
	schedule := DaySchedule{
		"thursday": {{Start: "07:00", End: "13:15"}},
	}
	tempAccess := []TempAccessRange{
		{
			Start: time.Date(2026, 2, 12, 16, 46, 0, 0, loc),
			End:   time.Date(2026, 2, 12, 17, 15, 0, 0, loc),
		},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, tempAccess, nil, false)
	// Should have: [17:00, 17:15] (temp access clipped to now at start)
	if len(intervals) != 1 {
		t.Fatalf("want 1 interval, got %d: %v", len(intervals), intervals)
	}
	expectStart := now
	expectEnd := time.Date(2026, 2, 12, 17, 15, 0, 0, loc)
	if !intervals[0].Start.Equal(expectStart) {
		t.Errorf("start: want %v, got %v", expectStart, intervals[0].Start)
	}
	if !intervals[0].End.Equal(expectEnd) {
		t.Errorf("end: want %v, got %v", expectEnd, intervals[0].End)
	}
}

func TestComputeAllowedIntervals_TempAccessAndSchedule(t *testing.T) {
	loc := time.UTC
	// Thursday 12 Feb 2026, 12:00 - during schedule
	now := time.Date(2026, 2, 12, 12, 0, 0, 0, loc)
	schedule := DaySchedule{
		"thursday": {{Start: "07:00", End: "13:15"}},
	}
	tempAccess := []TempAccessRange{
		{
			Start: time.Date(2026, 2, 12, 16, 0, 0, 0, loc),
			End:   time.Date(2026, 2, 12, 17, 0, 0, 0, loc),
		},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, tempAccess, nil, false)
	// Schedule: [12:00, 13:15] (clipped)
	// Temp: [16:00, 17:00]
	// Merged: [12:00, 13:15], [16:00, 17:00]
	if len(intervals) != 2 {
		t.Fatalf("want 2 intervals, got %d: %v", len(intervals), intervals)
	}
}

func TestComputeAllowedIntervals_BlockCutsSchedule(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 2, 12, 10, 0, 0, 0, loc)
	schedule := DaySchedule{
		"thursday": {{Start: "07:00", End: "13:15"}},
	}
	blocks := []BlockRange{
		{
			Start: time.Date(2026, 2, 12, 10, 0, 0, 0, loc),
			End:   time.Date(2026, 2, 12, 11, 0, 0, 0, loc),
		},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, nil, blocks, false)
	// [10:00, 13:15] cut by [10:00, 11:00] -> [11:00, 13:15]
	if len(intervals) != 1 {
		t.Fatalf("want 1 interval, got %d: %v", len(intervals), intervals)
	}
	expectStart := time.Date(2026, 2, 12, 11, 0, 0, 0, loc)
	expectEnd := time.Date(2026, 2, 12, 13, 15, 0, 0, loc)
	if !intervals[0].Start.Equal(expectStart) {
		t.Errorf("start: want %v, got %v", expectStart, intervals[0].Start)
	}
	if !intervals[0].End.Equal(expectEnd) {
		t.Errorf("end: want %v, got %v", expectEnd, intervals[0].End)
	}
}

func TestComputeAllowedIntervals_NoZeroLength(t *testing.T) {
	loc := time.UTC
	// Exactly at end of schedule
	now := time.Date(2026, 2, 12, 13, 15, 0, 0, loc)
	schedule := DaySchedule{
		"thursday": {{Start: "07:00", End: "13:15"}},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, nil, nil, false)
	if len(intervals) != 0 {
		t.Errorf("want 0 intervals (exact end time), got %d: %v", len(intervals), intervals)
	}
}

func TestComputeAllowedIntervals_OvernightSchedule(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 2, 12, 23, 0, 0, 0, loc)
	schedule := DaySchedule{
		"thursday": {{Start: "22:00", End: "02:00"}},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, nil, nil, false)
	if len(intervals) != 1 {
		t.Fatalf("want 1 interval, got %d", len(intervals))
	}
	expectStart := now
	expectEnd := time.Date(2026, 2, 13, 2, 0, 0, 0, loc)
	if !intervals[0].Start.Equal(expectStart) {
		t.Errorf("start: want %v, got %v", expectStart, intervals[0].Start)
	}
	if !intervals[0].End.Equal(expectEnd) {
		t.Errorf("end: want %v, got %v", expectEnd, intervals[0].End)
	}
}

func TestComputeAllowedIntervals_BlockCutsTempAccess(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 2, 12, 17, 0, 0, 0, loc)
	schedule := DaySchedule{"thursday": {}}
	tempAccess := []TempAccessRange{
		{
			Start: time.Date(2026, 2, 12, 16, 0, 0, 0, loc),
			End:   time.Date(2026, 2, 12, 18, 0, 0, 0, loc),
		},
	}
	blocks := []BlockRange{
		{
			Start: time.Date(2026, 2, 12, 17, 30, 0, 0, loc),
			End:   time.Date(2026, 2, 12, 17, 45, 0, 0, loc),
		},
	}
	intervals, _ := ComputeAllowedIntervals(now, schedule, tempAccess, blocks, false)
	// Temp clipped to now: [17:00, 18:00]
	// Block [17:30, 17:45] cuts it -> [17:00, 17:30], [17:45, 18:00]
	if len(intervals) != 2 {
		t.Fatalf("want 2 intervals, got %d: %v", len(intervals), intervals)
	}
}
