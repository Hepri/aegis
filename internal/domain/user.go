package domain

// User represents a controlled user account
type User struct {
	ID                  string
	Name                string
	Username            string // OS account name
	Schedule            DaySchedule
	EarnBalanceMinutes  int
	SolvedTaskIDs       []string
	EarnDayStats        EarnDayStats
	EarnLockedUntil     int64          // unix seconds; 0 = not locked
	ActiveEarnChallenge *EarnChallenge // unfinished question (anti-cheat sticky)
}
