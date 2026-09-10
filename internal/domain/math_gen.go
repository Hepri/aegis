package domain

import (
	"fmt"
	"math/rand"
	"strconv"
)

// GenerateMathGrade3 creates an early-3rd-grade math question (RU prompts).
// Covers: +/− within 1000, ×÷ tables 2–9, compare, missing addend.
func GenerateMathGrade3(rng *rand.Rand, rewardMinutes int) EarnChallenge {
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}
	if rewardMinutes <= 0 {
		rewardMinutes = DefaultEarnSettings().DefaultRewardMinutes
	}
	switch rng.Intn(6) {
	case 0:
		return genAdd(rng, rewardMinutes)
	case 1:
		return genSub(rng, rewardMinutes)
	case 2:
		return genMul(rng, rewardMinutes)
	case 3:
		return genDiv(rng, rewardMinutes)
	case 4:
		return genCompare(rng, rewardMinutes)
	default:
		return genMissingAddend(rng, rewardMinutes)
	}
}

func genAdd(rng *rand.Rand, reward int) EarnChallenge {
	a := rng.Intn(900) + 50
	b := rng.Intn(900) + 10
	if a+b > 1000 {
		b = rng.Intn(1000 - a)
	}
	ans := a + b
	return textChallenge(fmt.Sprintf("%d + %d = ?", a, b), ans, reward)
}

func genSub(rng *rand.Rand, reward int) EarnChallenge {
	a := rng.Intn(900) + 100
	b := rng.Intn(a)
	ans := a - b
	return textChallenge(fmt.Sprintf("%d − %d = ?", a, b), ans, reward)
}

func genMul(rng *rand.Rand, reward int) EarnChallenge {
	a := rng.Intn(8) + 2 // 2..9
	b := rng.Intn(9) + 1 // 1..9
	ans := a * b
	return textChallenge(fmt.Sprintf("%d × %d = ?", a, b), ans, reward)
}

func genDiv(rng *rand.Rand, reward int) EarnChallenge {
	b := rng.Intn(8) + 2 // 2..9
	q := rng.Intn(9) + 1 // 1..9
	a := b * q
	return textChallenge(fmt.Sprintf("%d ÷ %d = ?", a, b), q, reward)
}

func genCompare(rng *rand.Rand, reward int) EarnChallenge {
	a := rng.Intn(900) + 50
	b := rng.Intn(900) + 50
	var ans string
	switch {
	case a > b:
		ans = ">"
	case a < b:
		ans = "<"
	default:
		ans = "="
	}
	// Shuffle distractors but keep correct
	choices := shuffleChoices(rng, []string{">", "<", "="})
	return EarnChallenge{
		Prompt:        fmt.Sprintf("Сравни: %d □ %d", a, b),
		Answer:        ans,
		Choices:       choices,
		Kind:          EarnKindChoice,
		RewardMinutes: reward,
		Source:        EarnSourceGen,
	}
}

func genMissingAddend(rng *rand.Rand, reward int) EarnChallenge {
	a := rng.Intn(80) + 10
	x := rng.Intn(80) + 5
	sum := a + x
	return textChallenge(fmt.Sprintf("%d + ? = %d", a, sum), x, reward)
}

func textChallenge(prompt string, answer int, reward int) EarnChallenge {
	return EarnChallenge{
		Prompt:        prompt,
		Answer:        strconv.Itoa(answer),
		Kind:          EarnKindText,
		RewardMinutes: reward,
		Source:        EarnSourceGen,
	}
}

func shuffleChoices(rng *rand.Rand, in []string) []string {
	out := append([]string(nil), in...)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
