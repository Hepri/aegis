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

func TestCreditTowardBalanceCap(t *testing.T) {
	bal, got := CreditTowardBalanceCap(290, 20, 300)
	if bal != 300 || got != 10 {
		t.Fatalf("partial: bal=%d got=%d", bal, got)
	}
	bal, got = CreditTowardBalanceCap(300, 5, 300)
	if bal != 300 || got != 0 {
		t.Fatalf("full: bal=%d got=%d", bal, got)
	}
	bal, got = CreditTowardBalanceCap(10, 5, 300)
	if bal != 15 || got != 5 {
		t.Fatalf("under: bal=%d got=%d", bal, got)
	}
}

func TestRewardForSubject(t *testing.T) {
	s := DefaultEarnSettings()
	if s.RewardForSubject(SubjectMultiply) != 1 {
		t.Fatalf("multiply reward default=%d", s.RewardForSubject(SubjectMultiply))
	}
	if s.WrongPenaltyForSubject(SubjectMultiply) != 5 {
		t.Fatalf("multiply wrong default=%d", s.WrongPenaltyForSubject(SubjectMultiply))
	}
	s.SubjectRewardMinutes = map[string]int{SubjectMultiply: 3, SubjectMoral: 2}
	s.SubjectWrongPenaltyMinutes = map[string]int{SubjectMultiply: 7}
	s = NormalizeEarnSettings(s)
	if s.RewardForSubject(SubjectMultiply) != 3 {
		t.Fatalf("multiply=%d", s.RewardForSubject(SubjectMultiply))
	}
	if s.RewardForSubject(SubjectMath) != 1 {
		t.Fatalf("math default=%d", s.RewardForSubject(SubjectMath))
	}
	if s.WrongPenaltyForSubject(SubjectMultiply) != 7 {
		t.Fatalf("wrong=%d", s.WrongPenaltyForSubject(SubjectMultiply))
	}
	task := EarnTask{Subject: SubjectMoral}
	if EffectiveReward(task, s) != 2 {
		t.Fatalf("effective=%d", EffectiveReward(task, s))
	}
}

func TestSubjectRepeats(t *testing.T) {
	s := DefaultEarnSettings()
	if !s.SubjectRepeats(SubjectMoral) || !s.SubjectRepeats(SubjectMultiply) {
		t.Fatal("moral and multiply table should repeat by default")
	}
	if s.SubjectRepeats(SubjectMath) {
		t.Fatal("math should not repeat by default")
	}
	s.RepeatableSubjects = []string{SubjectMath, "правила дома"}
	s = NormalizeEarnSettings(s)
	if !s.SubjectRepeats(SubjectMath) || !s.SubjectRepeats("правила дома") {
		t.Fatalf("custom list: %+v", s.RepeatableSubjects)
	}
	if s.SubjectRepeats(SubjectMoral) {
		t.Fatal("moral not in explicit list")
	}
	s.RepeatableSubjects = []string{}
	s = NormalizeEarnSettings(s)
	if s.SubjectRepeats(SubjectMoral) {
		t.Fatal("explicit empty should disable all repeats")
	}
	legacy := ResolveEarnSettings(EarnSettings{DefaultRewardMinutes: 1, MaxEarnPerDay: 60, WrongLockSeconds: 10, WrongStreakLimit: 3})
	if !legacy.SubjectRepeats(SubjectMoral) || !legacy.SubjectRepeats(SubjectMultiply) {
		t.Fatal("legacy missing field should default to moral + multiply table")
	}
}
