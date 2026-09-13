package domain

import "testing"

func TestSubjectBankEnabled(t *testing.T) {
	s := DefaultEarnSettings()
	if !s.SubjectBankEnabled(SubjectEnglish) {
		t.Fatal("default should enable english")
	}
	s.DisabledSubjects = []string{SubjectEnglish, SubjectLiterature}
	s = NormalizeEarnSettings(s)
	if s.SubjectBankEnabled(SubjectEnglish) {
		t.Fatal("english should be disabled")
	}
	if !s.SubjectBankEnabled(SubjectMath) {
		t.Fatal("math bank should stay on")
	}
	if !s.SubjectBankEnabled("") {
		t.Fatal("custom empty subject stays on")
	}
	ch := EarnChallenge{ID: "en-1", Subject: SubjectEnglish, Source: EarnSourceBank}
	if s.ChallengeAllowed(ch) {
		t.Fatal("disabled bank challenge not allowed")
	}
	gen := EarnChallenge{ID: "gen-1", Source: EarnSourceGen}
	s.MathGeneratorEnabled = false
	if s.ChallengeAllowed(gen) {
		t.Fatal("disabled generator challenge not allowed")
	}
}
