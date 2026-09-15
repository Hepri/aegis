package domain

func moralBank() []EarnTask {
	return []EarnTask{
		bankTask("moral-001", SubjectMoral, "Нужно ли ныть?", "нет", []string{"да", "нет"}, 1),
		bankTask("moral-002", SubjectMoral, "Когда нужно делать уроки?", "вечером", []string{"утром", "вечером"}, 1),
	}
}
