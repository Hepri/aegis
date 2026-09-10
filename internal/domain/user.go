package domain

// User represents a controlled user account
type User struct {
	ID                 string
	Name               string
	Username           string // OS account name
	Schedule           DaySchedule
	EarnBalanceMinutes int
	SolvedTaskIDs      []string
	EarnDayStats       EarnDayStats
}
