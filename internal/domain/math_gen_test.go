package domain

import (
	"math/rand"
	"strconv"
	"testing"
)

func TestGenerateMathGrade3_AnswersMatch(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 300; i++ {
		ch := GenerateMathGrade3(rng, 1)
		if ch.Prompt == "" || ch.Answer == "" {
			t.Fatalf("empty challenge: %+v", ch)
		}
		if len(ch.Prompt) < 20 {
			t.Fatalf("prompt too short: %q", ch.Prompt)
		}
		if ch.Subject != SubjectMath {
			t.Fatalf("subject=%q", ch.Subject)
		}
		if _, err := strconv.Atoi(ch.Answer); err != nil {
			t.Fatalf("non-int answer %q for %q", ch.Answer, ch.Prompt)
		}
	}
}

func TestGenerateMathGrade3_DeterministicSeed(t *testing.T) {
	rng1 := rand.New(rand.NewSource(7))
	rng2 := rand.New(rand.NewSource(7))
	a := GenerateMathGrade3(rng1, 1)
	b := GenerateMathGrade3(rng2, 1)
	if a.Prompt != b.Prompt || a.Answer != b.Answer {
		t.Fatalf("expected same with same seed: %+v vs %+v", a, b)
	}
}

func TestBuiltInEarnBank_PerSubject100(t *testing.T) {
	bank := BuiltInEarnBank()
	bySubj := map[string]int{}
	for _, task := range bank {
		bySubj[task.Subject]++
		if task.ID == "" || task.Prompt == "" || task.Answer == "" {
			t.Fatalf("bad task: %+v", task)
		}
		if len(task.Choices) > 0 {
			minChoices, maxChoices := 10, 12
			if task.Subject == SubjectMoral {
				minChoices, maxChoices = 2, 4
			}
			if len(task.Choices) < minChoices || len(task.Choices) > maxChoices {
				t.Fatalf("%s choices=%d", task.ID, len(task.Choices))
			}
			found := false
			for _, c := range task.Choices {
				if c == task.Answer {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("answer missing in choices: %s", task.ID)
			}
		}
	}
	for _, need := range []string{SubjectEnglish, SubjectRussian, SubjectWorld, SubjectLiterature, SubjectMath} {
		if bySubj[need] != 100 {
			t.Fatalf("%s count=%d want 100", need, bySubj[need])
		}
	}
	if bySubj[SubjectMultiply] != 231 {
		t.Fatalf("%s count=%d want 231 (121× + 110÷)", SubjectMultiply, bySubj[SubjectMultiply])
	}
	if bySubj[SubjectMoral] != 2 {
		t.Fatalf("%s count=%d want 2", SubjectMoral, bySubj[SubjectMoral])
	}
}

func TestShuffleStrings_MovesFirst(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	in := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"}
	moved := false
	for i := 0; i < 20; i++ {
		out := ShuffleStrings(rng, in)
		if out[0] != "A" {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("shuffle never moved first element")
	}
}
