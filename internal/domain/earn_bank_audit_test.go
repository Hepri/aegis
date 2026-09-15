package domain

import (
	"fmt"
	"strings"
	"testing"
	"unicode"
)

func TestAuditAllChoiceTasks(t *testing.T) {
	authors := map[string]bool{
		"А. С. Пушкин": true, "И. А. Крылов": true, "К. И. Чуковский": true, "А. Л. Барто": true,
		"С. Я. Маршак": true, "Л. Н. Толстой": true, "Н. Н. Носов": true, "Э. Успенский": true,
		"В. Ю. Драгунский": true, "С. В. Михалков": true, "Астрид Линдгрен": true, "А. А. Милн": true,
		"Г. Х. Андерсен": true, "Ш. Перро": true, "Братья Гримм": true, "М. Ю. Лермонтов": true,
		"Н. А. Некрасов": true, "А. П. Чехов": true,
	}
	parts := map[string]bool{
		"существительное": true, "глагол": true, "прилагательное": true, "наречие": true,
		"местоимение": true, "числительное": true, "союз": true, "предлог": true,
		"частица": true, "междометие": true, "причастие": true, "деепричастие": true,
	}

	var bad []string
	for _, task := range BuiltInEarnBank() {
		if TaskKind(task) != EarnKindChoice {
			continue
		}
		if task.Subject == SubjectMoral {
			if len(task.Choices) < 2 {
				bad = append(bad, fmt.Sprintf("%s: %d choices", task.ID, len(task.Choices)))
				continue
			}
		} else if len(task.Choices) != 12 {
			bad = append(bad, fmt.Sprintf("%s: %d choices", task.ID, len(task.Choices)))
			continue
		}
		has := false
		for _, c := range task.Choices {
			if c == task.Answer {
				has = true
			}
			if strings.HasPrefix(c, "вариант ") {
				bad = append(bad, fmt.Sprintf("%s: filler %q", task.ID, c))
			}
			if strings.TrimSpace(c) == "" {
				bad = append(bad, fmt.Sprintf("%s: empty choice", task.ID))
			}
		}
		if !has {
			bad = append(bad, fmt.Sprintf("%s: missing answer", task.ID))
		}

		if task.Subject == SubjectEnglish && russianWordCount(task.Answer) >= 2 {
			for _, c := range task.Choices {
				if russianWordCount(c) < 2 {
					bad = append(bad, fmt.Sprintf("%s: short distractor %q for phrase %q", task.ID, c, task.Answer))
				}
			}
		}

		if authors[task.Answer] {
			for _, c := range task.Choices {
				if !authors[c] {
					bad = append(bad, fmt.Sprintf("%s: non-author %q", task.ID, c))
				}
			}
		}
		if parts[task.Answer] && strings.Contains(task.Prompt, "часть речи") {
			for _, c := range task.Choices {
				if !parts[c] {
					bad = append(bad, fmt.Sprintf("%s: non-POS %q", task.ID, c))
				}
			}
		}

		// World action answers (infinitive phrases) must not mix with bare topic nouns.
		if task.Subject == SubjectWorld && isWorldActionPhrase(task.Answer) {
			for _, c := range task.Choices {
				if isBareTopicNoun(c) {
					bad = append(bad, fmt.Sprintf("%s: nouny distractor %q among actions (ans %q)", task.ID, c, task.Answer))
				}
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d problems:\n%s", len(bad), strings.Join(bad[:min(80, len(bad))], "\n"))
	}
}

func isWorldActionPhrase(s string) bool {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return false
	}
	w := parts[0]
	switch w {
	case "часть", "место", "сеть", "кость", "масть", "повість":
		return false
	}
	for _, suf := range []string{"ать", "ять", "еть", "ить", "ти", "чь", "нуть"} {
		if strings.HasSuffix(w, suf) {
			return true
		}
	}
	return strings.HasSuffix(w, "ть") && !strings.HasSuffix(w, "сть")
}

func isBareTopicNoun(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, " ") {
		return false
	}
	switch s {
	case "да", "нет", "иногда", "никогда", "всегда":
		return false
	}
	for _, r := range s {
		if unicode.IsDigit(r) {
			return false
		}
	}
	// infinitive single word still "action-ish"
	for _, suf := range []string{"ть", "ти", "чь"} {
		if strings.HasSuffix(s, suf) {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
