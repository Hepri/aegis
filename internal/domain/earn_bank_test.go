package domain

import (
	"strings"
	"testing"
)

func TestBuiltInEarnBankChoiceQuality(t *testing.T) {
	bank := BuiltInEarnBank()
	if len(bank) < 400 {
		t.Fatalf("expected ~500 builtin tasks, got %d", len(bank))
	}

	authors := map[string]bool{
		"А. С. Пушкин": true, "И. А. Крылов": true, "К. И. Чуковский": true, "А. Л. Барто": true,
		"С. Я. Маршак": true, "Л. Н. Толстой": true, "Н. Н. Носов": true, "Э. Успенский": true,
		"В. Ю. Драгунский": true, "С. В. Михалков": true, "Астрид Линдгрен": true, "А. А. Милн": true,
		"Г. Х. Андерсен": true, "Ш. Перро": true, "Братья Гримм": true, "М. Ю. Лермонтов": true,
		"Н. А. Некрасов": true, "А. П. Чехов": true,
	}
	partsOfSpeech := map[string]bool{
		"существительное": true, "глагол": true, "прилагательное": true, "наречие": true,
		"местоимение": true, "числительное": true, "союз": true, "предлог": true,
		"частица": true, "междометие": true, "причастие": true, "деепричастие": true,
	}

	var checkedAuthors, checkedParts int
	for _, task := range bank {
		if task.Kind != EarnKindChoice {
			continue
		}
		if len(task.Choices) != 12 {
			t.Errorf("%s: want 12 choices, got %d", task.ID, len(task.Choices))
		}
		hasAnswer := false
		for _, c := range task.Choices {
			if c == task.Answer {
				hasAnswer = true
			}
			if strings.HasPrefix(c, "вариант ") {
				t.Errorf("%s: padded filler choice %q (pool too small / mixed)", task.ID, c)
			}
		}
		if !hasAnswer {
			t.Errorf("%s: answer %q missing from choices", task.ID, task.Answer)
		}

		if authors[task.Answer] {
			checkedAuthors++
			for _, c := range task.Choices {
				if !authors[c] {
					t.Errorf("%s (%q): distractor %q is not an author", task.ID, task.Prompt, c)
				}
			}
		}
		if partsOfSpeech[task.Answer] && strings.Contains(task.Prompt, "часть речи") {
			checkedParts++
			for _, c := range task.Choices {
				if !partsOfSpeech[c] {
					t.Errorf("%s: distractor %q is not a part of speech", task.ID, c)
				}
			}
		}
	}
	if checkedAuthors < 20 {
		t.Fatalf("expected many author questions, checked %d", checkedAuthors)
	}
	if checkedParts < 10 {
		t.Fatalf("expected many part-of-speech questions, checked %d", checkedParts)
	}

	// Spot-check the screenshot case: dead princess author must only offer authors.
	var found bool
	for _, task := range bank {
		if task.ID != "lit-024" && !strings.Contains(task.Prompt, "мёртвой царевне") {
			continue
		}
		found = true
		if task.Answer != "А. С. Пушкин" {
			t.Errorf("dead princess answer = %q, want Пушкин", task.Answer)
		}
		for _, c := range task.Choices {
			if !authors[c] {
				t.Errorf("lit dead-princess has non-author choice %q", c)
			}
		}
	}
	if !found {
		t.Fatal("dead princess question not found")
	}

	// English: multi-word translations must not sit next to bare nouns like «ухо».
	var likeApples bool
	for _, task := range bank {
		if task.Subject != SubjectEnglish || task.Kind != EarnKindChoice {
			continue
		}
		for _, c := range task.Choices {
			if !choiceLooksLikeSameKind(task.Answer, c) {
				t.Errorf("%s (%q): choice %q does not match answer shape %q", task.ID, task.Prompt, c, task.Answer)
			}
		}
		if strings.Contains(task.Prompt, "I like apples") {
			likeApples = true
			if task.Answer != "я люблю яблоки" {
				t.Errorf("I like apples answer = %q", task.Answer)
			}
			for _, c := range task.Choices {
				if russianWordCount(c) < 2 {
					t.Errorf("I like apples has short distractor %q", c)
				}
			}
		}
	}
	if !likeApples {
		t.Fatal("I like apples question not found")
	}
}
