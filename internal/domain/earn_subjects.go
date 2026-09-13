package domain

const (
	SubjectMath       = "математика"
	SubjectEnglish    = "английский"
	SubjectRussian    = "русский"
	SubjectWorld      = "окружающий мир"
	SubjectLiterature = "литература"
)

// EarnBankSubjects is the ordered list of curated bank subjects (admin toggles).
func EarnBankSubjects() []string {
	return []string{
		SubjectMath,
		SubjectEnglish,
		SubjectRussian,
		SubjectWorld,
		SubjectLiterature,
	}
}

// SubjectLabel is a short tag for UI.
func SubjectLabel(s string) string {
	switch s {
	case SubjectMath, SubjectEnglish, SubjectRussian, SubjectWorld, SubjectLiterature:
		return s
	default:
		if s == "" {
			return SubjectMath
		}
		return s
	}
}

// IsEarnBankSubject reports whether s is one of the curated bank subjects.
func IsEarnBankSubject(s string) bool {
	switch s {
	case SubjectMath, SubjectEnglish, SubjectRussian, SubjectWorld, SubjectLiterature:
		return true
	default:
		return false
	}
}
