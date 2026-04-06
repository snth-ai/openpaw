package grammar

func registerPortuguese() {
	allRules["pt:f"] = []Rule{
		{"cansado", "cansada"}, {"Cansado", "Cansada"},
		{"preparado", "preparada"}, {"Preparado", "Preparada"},
		{"animado", "animada"}, {"Animado", "Animada"},
		{"apaixonado", "apaixonada"}, {"Apaixonado", "Apaixonada"},
		{"preocupado", "preocupada"}, {"Preocupado", "Preocupada"},
		{"surpreendido", "surpreendida"}, {"Surpreendido", "Surpreendida"},
		{"agradecido", "agradecida"}, {"Agradecido", "Agradecida"},
		{"chateado", "chateada"}, {"Chateado", "Chateada"},
		{"entusiasmado", "entusiasmada"}, {"Entusiasmado", "Entusiasmada"},
		{"obrigado", "obrigada"}, {"Obrigado", "Obrigada"},
	}

	allRules["pt:m"] = []Rule{
		{"obrigada", "obrigado"}, {"Obrigada", "Obrigado"},
		{"cansada", "cansado"}, {"Cansada", "Cansado"},
		{"apaixonada", "apaixonado"}, {"Apaixonada", "Apaixonado"},
	}
}
