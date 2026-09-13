package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func englishBank() []EarnTask {
	type item struct {
		en, ru string
		pool   []string
	}

	// Each pool is only plausible Russian translations of the same kind.
	bePool := []string{
		"я", "ты", "вы", "он", "она", "мы", "они", "это", "оно",
		"я есть", "ты есть", "он есть", "она есть", "мы есть", "они есть",
	}
	havePool := []string{
		"у меня есть", "у тебя есть", "у него есть", "у неё есть",
		"у нас есть", "у них есть", "у меня нет", "у тебя нет",
		"у него нет", "у неё нет", "есть книга", "нет книги",
	}
	verbMePool := []string{
		"мне нравится", "я люблю", "я вижу", "я иду", "я могу", "я хочу",
		"мне нужно", "я не знаю", "я не понимаю", "я умею", "я читаю", "я пишу",
	}
	greetPhrasePool := []string{
		"доброе утро", "спокойной ночи", "как дела?", "у меня всё хорошо",
		"как тебя зовут?", "меня зовут", "приятно познакомиться",
		"пожалуйста (на здоровье)", "до встречи", "с днём рождения",
		"с рождеством", "с новым годом", "без проблем", "до свидания",
	}
	greetShortPool := []string{
		"спасибо", "извините", "извини", "конечно", "привет", "пока",
		"пожалуйста", "отлично", "ладно", "хорошо", "стоп", "старт",
	}
	cmdPool := []string{
		"иди сюда", "садись пожалуйста", "встань пожалуйста", "открой книгу", "закрой дверь",
		"посмотри на меня", "послушай меня", "помоги мне", "пойдём скорее",
		"помой руки", "почисти зубы", "подожди минуту", "поверни налево",
		"поверни направо", "иди прямо", "остановись здесь", "открой окно",
		"закрой книгу", "смотри сюда", "слушай папу", "иди домой", "будь тише",
	}
	weatherPool := []string{
		"жарко", "холодно", "солнечно", "пасмурно", "ветрено", "дождливо",
		"снежно", "тепло", "прохладно", "душно", "сыро", "сухо",
	}
	sentencePool := []string{
		"я мальчик", "я девочка", "это кошка", "то собака", "я счастлив", "мне грустно",
		"идёт дождь", "я иду в школу", "я иду домой",
		"я играю в футбол", "я читаю книгу", "я пишу письмо", "где это?", "что это?",
		"сколько тебе лет?", "мне восемь", "мне девять", "мне десять",
		"я голоден", "я хочу пить", "я люблю яблоки", "я люблю собак",
		"она моя мама", "он мой папа", "мы друзья", "они учителя",
		"солнце жёлтое", "небо синее", "я умею плавать", "я умею бегать",
		"я умею читать", "я умею писать", "я не хочу", "он идёт", "мы играем",
		"я вижу дом", "мне холодно", "мне жарко",
	}
	nounPhrasePool := []string{
		"красное яблоко", "большой дом", "маленькая кошка", "мой друг", "твоя книга",
		"его ручка", "её сумка", "наша школа", "их мяч", "в комнате", "на столе",
		"под стулом", "в школе", "возле дома", "синее небо", "жёлтое солнце",
		"новый мяч", "старый дом", "быстрая машина", "маленький пёс",
	}
	nounPool := []string{
		"яблоко", "собака", "кошка", "дом", "школа", "книга", "ручка", "вода",
		"молоко", "хлеб", "солнце", "луна", "дождь", "снег", "мама", "папа",
		"друг", "учитель", "утро", "ночь", "зима", "лето", "весна", "осень",
		"стол", "стул", "окно", "дверь", "птица", "рыба", "мальчик", "девочка",
		"мяч", "дерево", "цветок", "машина", "автобус", "поезд", "велосипед",
		"телефон", "компьютер", "сумка", "шляпа", "пальто", "туфля", "носок",
		"рука", "нога", "голова", "глаз", "ухо", "нос", "рот", "имя", "семья",
		"брат", "сестра", "малыш", "еда", "сок", "чай", "кофе", "яйцо", "сыр",
		"сад", "лес", "река", "море", "гора", "город",
	}
	adjPool := []string{
		"красный", "синий", "зелёный", "жёлтый", "большой", "маленький",
		"счастливый", "грустный", "горячий", "холодный", "хороший", "плохой",
		"новый", "старый", "быстрый", "медленный",
	}
	verbPool := []string{"бегать", "читать", "писать", "прыгать", "плавать", "играть", "смотреть", "слушать", "идти", "есть", "пить", "спать"}
	numPool := []string{"один", "два", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять", "десять", "ноль", "двенадцать"}
	shortPool := []string{"да", "нет", "пожалуйста", "спасибо", "привет", "пока", "хорошо", "плохо", "стоп", "старт", "ок", "приветствие"}

	items := []item{
		{"I am", "я", bePool}, {"you are", "ты", bePool}, {"he is", "он", bePool},
		{"she is", "она", bePool}, {"it is", "это", bePool}, {"we are", "мы", bePool},
		{"they are", "они", bePool},
		{"I have", "у меня есть", havePool}, {"you have", "у тебя есть", havePool},
		{"he has", "у него есть", havePool}, {"she has", "у неё есть", havePool},
		{"I like", "мне нравится", verbMePool}, {"I love", "я люблю", verbMePool},
		{"I see", "я вижу", verbMePool}, {"I go", "я иду", verbMePool},
		{"I can", "я могу", verbMePool}, {"I want", "я хочу", verbMePool},
		{"I need", "мне нужно", verbMePool},
		{"good morning", "доброе утро", greetPhrasePool}, {"good night", "спокойной ночи", greetPhrasePool},
		{"how are you?", "как дела?", greetPhrasePool}, {"I am fine", "у меня всё хорошо", greetPhrasePool},
		{"what is your name?", "как тебя зовут?", greetPhrasePool}, {"my name is", "меня зовут", greetPhrasePool},
		{"nice to meet you", "приятно познакомиться", greetPhrasePool},
		{"thank you", "спасибо", greetShortPool},
		{"you are welcome", "пожалуйста (на здоровье)", greetPhrasePool}, {"see you", "до встречи", greetPhrasePool},
		{"come here", "иди сюда", cmdPool}, {"sit down", "садись пожалуйста", cmdPool}, {"stand up", "встань пожалуйста", cmdPool},
		{"open the book", "открой книгу", cmdPool}, {"close the door", "закрой дверь", cmdPool},
		{"look at me", "посмотри на меня", cmdPool}, {"listen to me", "послушай меня", cmdPool},
		{"help me", "помоги мне", cmdPool}, {"let's go", "пойдём скорее", cmdPool}, {"be quiet", "будь тише", cmdPool},
		{"wash your hands", "помой руки", cmdPool}, {"brush your teeth", "почисти зубы", cmdPool},
		{"wait a minute", "подожди минуту", cmdPool}, {"turn left", "поверни налево", cmdPool},
		{"turn right", "поверни направо", cmdPool}, {"go straight", "иди прямо", cmdPool},
		{"stop here", "остановись здесь", cmdPool},
		{"I am a boy", "я мальчик", sentencePool}, {"I am a girl", "я девочка", sentencePool},
		{"this is a cat", "это кошка", sentencePool}, {"that is a dog", "то собака", sentencePool},
		{"I am happy", "я счастлив", sentencePool}, {"I am sad", "мне грустно", sentencePool},
		{"it is hot", "жарко", weatherPool}, {"it is cold", "холодно", weatherPool},
		{"it is sunny", "солнечно", weatherPool}, {"it is raining", "идёт дождь", sentencePool},
		{"I go to school", "я иду в школу", sentencePool}, {"I go home", "я иду домой", sentencePool},
		{"I play football", "я играю в футбол", sentencePool}, {"I read a book", "я читаю книгу", sentencePool},
		{"I write a letter", "я пишу письмо", sentencePool},
		{"where is it?", "где это?", sentencePool}, {"what is this?", "что это?", sentencePool},
		{"how old are you?", "сколько тебе лет?", sentencePool},
		{"I am eight", "мне восемь", sentencePool}, {"I am nine", "мне девять", sentencePool},
		{"I am ten", "мне десять", sentencePool},
		{"I am hungry", "я голоден", sentencePool}, {"I am thirsty", "я хочу пить", sentencePool},
		{"I like apples", "я люблю яблоки", sentencePool}, {"I like dogs", "я люблю собак", sentencePool},
		{"she is my mother", "она моя мама", sentencePool}, {"he is my father", "он мой папа", sentencePool},
		{"we are friends", "мы друзья", sentencePool}, {"they are teachers", "они учителя", sentencePool},
		{"the sun is yellow", "солнце жёлтое", sentencePool}, {"the sky is blue", "небо синее", sentencePool},
		{"I can swim", "я умею плавать", sentencePool}, {"I can run", "я умею бегать", sentencePool},
		{"I can read", "я умею читать", sentencePool}, {"I can write", "я умею писать", sentencePool},
		{"I don't know", "я не знаю", sentencePool}, {"I don't understand", "я не понимаю", sentencePool},
		{"happy birthday", "с днём рождения", greetPhrasePool}, {"merry christmas", "с рождеством", greetPhrasePool},
		{"happy new year", "с новым годом", greetPhrasePool}, {"excuse me", "извините", greetShortPool},
		{"I am sorry", "извини", greetShortPool}, {"of course", "конечно", greetShortPool},
		{"no problem", "без проблем", greetPhrasePool},
		{"red apple", "красное яблоко", nounPhrasePool}, {"big house", "большой дом", nounPhrasePool},
		{"small cat", "маленькая кошка", nounPhrasePool}, {"my friend", "мой друг", nounPhrasePool},
		{"your book", "твоя книга", nounPhrasePool}, {"his pen", "его ручка", nounPhrasePool},
		{"her bag", "её сумка", nounPhrasePool}, {"our school", "наша школа", nounPhrasePool},
		{"their ball", "их мяч", nounPhrasePool}, {"in the room", "в комнате", nounPhrasePool},
		{"on the table", "на столе", nounPhrasePool}, {"under the chair", "под стулом", nounPhrasePool},
		{"at school", "в школе", nounPhrasePool}, {"at home", "я дома", sentencePool},
		{"apple", "яблоко", nounPool}, {"dog", "собака", nounPool}, {"cat", "кошка", nounPool},
		{"house", "дом", nounPool}, {"school", "школа", nounPool}, {"book", "книга", nounPool},
		{"pen", "ручка", nounPool}, {"water", "вода", nounPool}, {"milk", "молоко", nounPool},
		{"bread", "хлеб", nounPool}, {"sun", "солнце", nounPool}, {"moon", "луна", nounPool},
		{"mother", "мама", nounPool}, {"father", "папа", nounPool}, {"friend", "друг", nounPool},
		{"teacher", "учитель", nounPool}, {"boy", "мальчик", nounPool}, {"girl", "девочка", nounPool},
		{"red", "красный", adjPool}, {"blue", "синий", adjPool}, {"green", "зелёный", adjPool},
		{"yellow", "жёлтый", adjPool}, {"big", "большой", adjPool}, {"small", "маленький", adjPool},
		{"happy", "счастливый", adjPool}, {"sad", "грустный", adjPool}, {"hot", "горячий", adjPool},
		{"cold", "холодный", adjPool}, {"good", "хороший", adjPool}, {"bad", "плохой", adjPool},
		{"run", "бегать", verbPool}, {"read", "читать", verbPool}, {"write", "писать", verbPool},
		{"jump", "прыгать", verbPool},
		{"one", "один", numPool}, {"two", "два", numPool}, {"three", "три", numPool},
		{"four", "четыре", numPool}, {"five", "пять", numPool}, {"ten", "десять", numPool},
		{"yes", "да", shortPool}, {"no", "нет", shortPool}, {"hello", "привет", shortPool},
		{"please", "пожалуйста", shortPool}, {"thanks", "спасибо", shortPool},
	}

	out := make([]EarnTask, 0, 100)
	for i, it := range items {
		if i >= 100 {
			break
		}
		out = append(out, choiceTask(
			fmt.Sprintf("en-%03d", i+1),
			SubjectEnglish,
			fmt.Sprintf("Переведи на русский: «%s»", it.en),
			it.ru,
			it.pool,
			1,
		))
	}
	return out
}

// choiceLooksLikeSameKind: multi-word answers must not sit next to bare one-word nouns.
func choiceLooksLikeSameKind(answer, choice string) bool {
	if russianWordCount(answer) >= 2 {
		return russianWordCount(choice) >= 2
	}
	return true
}

func russianWordCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := 0
	for _, p := range strings.Fields(s) {
		p = strings.Trim(p, ".,!?()«»\"'")
		if p == "" || p == "/" {
			continue
		}
		if utf8.RuneCountInString(p) > 0 {
			n++
		}
	}
	return n
}
