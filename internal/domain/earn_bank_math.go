package domain

import (
	"fmt"
	"math/rand"
)

// mathBank returns 100 fixed word problems (deterministic) for admin browsing and rotation.
func mathBank() []EarnTask {
	rng := rand.New(rand.NewSource(20260911))
	out := make([]EarnTask, 0, 100)
	for i := 0; i < 100; i++ {
		ch := GenerateMathGrade3(rng, 1)
		out = append(out, EarnTask{
			ID:            fmt.Sprintf("math-%03d", i+1),
			Prompt:        ch.Prompt,
			Answer:        ch.Answer,
			Kind:          EarnKindText,
			Subject:       SubjectMath,
			RewardMinutes: 1,
			Enabled:       true,
		})
	}
	return out
}
