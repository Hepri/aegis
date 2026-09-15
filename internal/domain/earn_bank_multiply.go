package domain

import "fmt"

// multiplyBank is 0..10 × 0..10 multiplication plus matching division (no ÷0).
func multiplyBank() []EarnTask {
	out := make([]EarnTask, 0, 250)
	n := 0
	for a := 0; a <= 10; a++ {
		for b := 0; b <= 10; b++ {
			n++
			prod := a * b
			out = append(out, bankTask(
				fmt.Sprintf("mul-%03d", n),
				SubjectMultiply,
				fmt.Sprintf("Сколько будет %d × %d?", a, b),
				fmt.Sprintf("%d", prod),
				nil,
				1,
			))
		}
	}
	// Деление: (делитель × частное) ÷ делитель = частное; частное может быть 0.
	n = 0
	for divisor := 1; divisor <= 10; divisor++ {
		for quot := 0; quot <= 10; quot++ {
			n++
			dividend := divisor * quot
			out = append(out, bankTask(
				fmt.Sprintf("div-%03d", n),
				SubjectMultiply,
				fmt.Sprintf("Сколько будет %d ÷ %d?", dividend, divisor),
				fmt.Sprintf("%d", quot),
				nil,
				1,
			))
		}
	}
	return out
}
