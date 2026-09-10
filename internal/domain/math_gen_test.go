package domain

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func TestGenerateMathGrade3_AnswersMatch(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		ch := GenerateMathGrade3(rng, 1)
		if ch.Prompt == "" || ch.Answer == "" {
			t.Fatalf("empty challenge: %+v", ch)
		}
		if ch.Kind == EarnKindChoice {
			found := false
			for _, c := range ch.Choices {
				if c == ch.Answer {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("answer %q not in choices %v for %q", ch.Answer, ch.Choices, ch.Prompt)
			}
		} else {
			if _, err := strconv.Atoi(ch.Answer); err != nil {
				t.Fatalf("non-int answer %q for %q", ch.Answer, ch.Prompt)
			}
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
	_ = time.Now()
}
