package domain

import (
	"fmt"
	"math/rand"
)

// BuiltInEarnBank returns the curated grade-3 task pack shipped with the server.
func BuiltInEarnBank() []EarnTask {
	var out []EarnTask
	out = append(out, englishBank()...)
	out = append(out, russianBank()...)
	out = append(out, worldBank()...)
	out = append(out, literatureBank()...)
	out = append(out, mathBank()...)
	return out
}

func bankTask(id, subject, prompt, answer string, choices []string, reward int) EarnTask {
	kind := EarnKindText
	if len(choices) > 0 {
		kind = EarnKindChoice
	}
	if reward <= 0 {
		reward = 1
	}
	return EarnTask{
		ID:            id,
		Prompt:        prompt,
		Answer:        answer,
		Choices:       append([]string(nil), choices...),
		Kind:          kind,
		Subject:       subject,
		RewardMinutes: reward,
		Enabled:       true,
	}
}

// choiceTask builds a multiple-choice item with 12 options (answer + 11 distractors).
func choiceTask(id, subject, prompt, answer string, distractorPool []string, reward int) EarnTask {
	seen := map[string]bool{answer: true}
	pool := make([]string, 0, len(distractorPool))
	for _, d := range distractorPool {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		pool = append(pool, d)
	}
	rng := rand.New(rand.NewSource(hashID(id)))
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	need := 11
	if len(pool) < need {
		for len(pool) < need {
			pool = append(pool, fmt.Sprintf("вариант %d", len(pool)+1))
		}
	}
	choices := append([]string{answer}, pool[:need]...)
	rng.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
	return bankTask(id, subject, prompt, answer, choices, reward)
}

func hashID(id string) int64 {
	var h int64 = 17
	for _, c := range id {
		h = h*31 + int64(c)
	}
	if h < 0 {
		h = -h
	}
	return h + 1
}

// MergeEarnBanks concatenates builtin + client custom tasks (client last).
func MergeEarnBanks(clientTasks []EarnTask) []EarnTask {
	base := BuiltInEarnBank()
	if len(clientTasks) == 0 {
		return base
	}
	seen := map[string]bool{}
	for _, t := range base {
		seen[t.ID] = true
	}
	out := append([]EarnTask{}, base...)
	for _, t := range clientTasks {
		if t.ID == "" || seen[t.ID] {
			continue
		}
		out = append(out, t)
		seen[t.ID] = true
	}
	return out
}
