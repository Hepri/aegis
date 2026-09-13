package domain

import (
	"fmt"
	"math/rand"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// GenerateMathGrade3 creates a harder early/mid 3rd-grade Russian word problem
// (two-step stories, larger numbers, tables 2–9).
func GenerateMathGrade3(rng *rand.Rand, rewardMinutes int) EarnChallenge {
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}
	if rewardMinutes <= 0 {
		rewardMinutes = DefaultEarnSettings().DefaultRewardMinutes
	}
	gens := []func(*rand.Rand, int) EarnChallenge{
		genHardAddChain,
		genHardSubThenAdd,
		genHardBuyTwoItems,
		genHardDiffThenTotal,
		genHardMulThenSub,
		genHardShareRemainder,
		genHardRowsColsExtra,
		genHardTrips,
		genHardCompareTriple,
		genHardPackFactory,
		genHardAgeStory,
		genHardBusTwoStops,
	}
	ch := gens[rng.Intn(len(gens))](rng, rewardMinutes)
	ch.Subject = SubjectMath
	return ch
}

var (
	names = []string{
		"Вася", "Петя", "Коля", "Миша", "Саша", "Дима", "Артём", "Лёша",
		"Маша", "Даша", "Катя", "Лена", "Аня", "Оля", "Соня", "Настя", "Юля", "Ира",
	}
)

type noun struct {
	One  string
	Few  string
	Many string
}

func (n noun) count(k int) string {
	return fmt.Sprintf("%d %s", k, n.form(k))
}

func (n noun) form(k int) string {
	mod10 := k % 10
	mod100 := k % 100
	if mod10 == 1 && mod100 != 11 {
		return n.One
	}
	if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
		return n.Few
	}
	return n.Many
}

var inventory = []noun{
	{"яблоко", "яблока", "яблок"},
	{"груша", "груши", "груш"},
	{"конфета", "конфеты", "конфет"},
	{"карандаш", "карандаша", "карандашей"},
	{"наклейка", "наклейки", "наклеек"},
	{"маркер", "маркера", "маркеров"},
	{"машинка", "машинки", "машинок"},
	{"книга", "книги", "книг"},
	{"открытка", "открытки", "открыток"},
	{"шар", "шара", "шаров"},
	{"марка", "марки", "марок"},
	{"орех", "ореха", "орехов"},
	{"цветок", "цветка", "цветков"},
	{"камешек", "камешка", "камешков"},
	{"ракушка", "ракушки", "ракушек"},
	{"значок", "значка", "значков"},
	{"лист", "листа", "листов"},
	{"мяч", "мяча", "мячей"},
	{"тетрадь", "тетради", "тетрадей"},
	{"ручка", "ручки", "ручек"},
}

func pickThing(rng *rand.Rand) noun   { return inventory[rng.Intn(len(inventory))] }
func pickName(rng *rand.Rand) string  { return names[rng.Intn(len(names))] }
func otherName(rng *rand.Rand, avoid string) string {
	for i := 0; i < 12; i++ {
		n := pickName(rng)
		if n != avoid {
			return n
		}
	}
	return pickName(rng)
}

func genHardAddChain(rng *rand.Rand, reward int) EarnChallenge {
	who := pickName(rng)
	item := pickThing(rng)
	a := rng.Intn(180) + 40
	b := rng.Intn(160) + 30
	c := rng.Intn(120) + 20
	prompt := fmt.Sprintf("У %s было %s. Потом нашли ещё %s и купили %s. Сколько %s стало?",
		who, item.count(a), item.count(b), item.count(c), item.Many)
	return textChallenge(prompt, a+b+c, reward)
}

func genHardSubThenAdd(rng *rand.Rand, reward int) EarnChallenge {
	who := pickName(rng)
	item := pickThing(rng)
	start := rng.Intn(250) + 120
	gave := rng.Intn(60) + 20
	got := rng.Intn(80) + 15
	if gave >= start {
		gave = start / 3
	}
	prompt := fmt.Sprintf("У %s было %s. %s отдал другу %s, а потом получил в подарок %s. Сколько %s теперь?",
		who, item.count(start), who, item.count(gave), item.count(got), item.Many)
	return textChallenge(prompt, start-gave+got, reward)
}

func genHardBuyTwoItems(rng *rand.Rand, reward int) EarnChallenge {
	item1 := pickThing(rng)
	item2 := pickThing(rng)
	for item2.One == item1.One {
		item2 = pickThing(rng)
	}
	p1 := rng.Intn(40) + 10
	n1 := rng.Intn(6) + 2
	p2 := rng.Intn(40) + 10
	n2 := rng.Intn(6) + 2
	had := p1*n1 + p2*n2 + rng.Intn(50) + 20
	prompt := fmt.Sprintf("Было %d рублей. Купили %s по %d ₽ и %s по %d ₽. Сколько рублей осталось?",
		had, item1.count(n1), p1, item2.count(n2), p2)
	return textChallenge(prompt, had-p1*n1-p2*n2, reward)
}

func genHardDiffThenTotal(rng *rand.Rand, reward int) EarnChallenge {
	aName := pickName(rng)
	bName := otherName(rng, aName)
	item := pickThing(rng)
	small := rng.Intn(80) + 30
	diff := rng.Intn(70) + 20
	big := small + diff
	prompt := fmt.Sprintf("У %s %s. У %s на %s больше. Сколько %s у них вместе?",
		aName, item.count(small), bName, item.count(diff), item.Many)
	return textChallenge(prompt, small+big, reward)
}

func genHardMulThenSub(rng *rand.Rand, reward int) EarnChallenge {
	item := pickThing(rng)
	boxes := rng.Intn(7) + 3
	each := rng.Intn(8) + 2
	took := rng.Intn(boxes*each/2) + 3
	total := boxes * each
	if took >= total {
		took = total / 3
	}
	prompt := fmt.Sprintf("В %d коробках по %s. Из них взяли %s. Сколько %s осталось?",
		boxes, item.count(each), item.count(took), item.Many)
	return textChallenge(prompt, total-took, reward)
}

func genHardShareRemainder(rng *rand.Rand, reward int) EarnChallenge {
	who := pickName(rng)
	item := pickThing(rng)
	kids := rng.Intn(6) + 3
	each := rng.Intn(8) + 2
	extra := rng.Intn(kids-1) + 1
	total := kids*each + extra
	prompt := fmt.Sprintf("%s раздал %s %d детям поровну, и ещё осталось %s. По сколько %s получил каждый?",
		who, item.count(total), kids, item.count(extra), item.Many)
	return textChallenge(prompt, each, reward)
}

func genHardRowsColsExtra(rng *rand.Rand, reward int) EarnChallenge {
	item := pickThing(rng)
	rows := rng.Intn(6) + 3
	cols := rng.Intn(7) + 3
	extra := rng.Intn(20) + 5
	prompt := fmt.Sprintf("%s расставили в %d ряда по %s, и отдельно лежит ещё %s. Сколько всего %s?",
		capitalize(item.Many), rows, item.count(cols), item.count(extra), item.Many)
	return textChallenge(prompt, rows*cols+extra, reward)
}

func genHardTrips(rng *rand.Rand, reward int) EarnChallenge {
	who := pickName(rng)
	item := pickThing(rng)
	trips := rng.Intn(5) + 3
	each := rng.Intn(40) + 15
	prompt := fmt.Sprintf("%s %d раза относил на склад по %s. Сколько всего %s он отнёс?",
		who, trips, item.count(each), item.Many)
	return textChallenge(prompt, trips*each, reward)
}

func genHardCompareTriple(rng *rand.Rand, reward int) EarnChallenge {
	a := pickName(rng)
	b := otherName(rng, a)
	c := otherName(rng, b)
	for c == a {
		c = otherName(rng, b)
	}
	item := pickThing(rng)
	x := rng.Intn(50) + 20
	y := x + rng.Intn(30) + 5
	z := y + rng.Intn(30) + 5
	prompt := fmt.Sprintf("У %s %s, у %s — %s, у %s — %s. На сколько %s у %s больше, чем у %s?",
		a, item.count(x), b, item.count(y), c, item.count(z), item.Many, c, a)
	return textChallenge(prompt, z-x, reward)
}

func genHardPackFactory(rng *rand.Rand, reward int) EarnChallenge {
	item := pickThing(rng)
	packs := rng.Intn(8) + 4
	each := rng.Intn(9) + 2
	broken := rng.Intn(packs*each/4) + 2
	total := packs * each
	if broken >= total {
		broken = total / 5
	}
	prompt := fmt.Sprintf("Собрали %d пакетов по %s. %s оказалось бракованными. Сколько хороших %s осталось?",
		packs, item.count(each), item.count(broken), item.Many)
	return textChallenge(prompt, total-broken, reward)
}

func genHardAgeStory(rng *rand.Rand, reward int) EarnChallenge {
	a := pickName(rng)
	b := otherName(rng, a)
	young := rng.Intn(5) + 8
	diff := rng.Intn(6) + 3
	years := rng.Intn(4) + 2
	prompt := fmt.Sprintf("%s сейчас %d лет, а %s старше на %d года. Сколько лет будет %s через %d года?",
		a, young, b, diff, b, years)
	return textChallenge(prompt, young+diff+years, reward)
}

func genHardBusTwoStops(rng *rand.Rand, reward int) EarnChallenge {
	start := rng.Intn(25) + 15
	off1 := rng.Intn(8) + 2
	on1 := rng.Intn(10) + 3
	off2 := rng.Intn(8) + 2
	on2 := rng.Intn(10) + 2
	mid := start - off1 + on1
	if off2 >= mid {
		off2 = mid / 2
	}
	prompt := fmt.Sprintf("В автобусе было %d пассажиров. На первой остановке вышло %d и вошло %d, на второй вышло %d и вошло %d. Сколько пассажиров стало?",
		start, off1, on1, off2, on2)
	return textChallenge(prompt, mid-off2+on2, reward)
}

func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func textChallenge(prompt string, answer int, reward int) EarnChallenge {
	return EarnChallenge{
		Prompt:        prompt,
		Answer:        strconv.Itoa(answer),
		Kind:          EarnKindText,
		Subject:       SubjectMath,
		RewardMinutes: reward,
		Source:        EarnSourceGen,
	}
}
