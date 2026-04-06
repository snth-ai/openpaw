package grammar

func registerFrench() {
	allRules["fr:f"] = []Rule{
		{"fatigué", "fatiguée"}, {"Fatigué", "Fatiguée"},
		{"préparé", "préparée"}, {"Préparé", "Préparée"},
		{"surpris", "surprise"}, {"Surpris", "Surprise"},
		{"content", "contente"}, {"Content", "Contente"},
		{"prêt", "prête"}, {"Prêt", "Prête"},
		{"heureux", "heureuse"}, {"Heureux", "Heureuse"},
		{"amoureux", "amoureuse"}, {"Amoureux", "Amoureuse"},
		{"jaloux", "jalouse"}, {"Jaloux", "Jalouse"},
		{"désolé", "désolée"}, {"Désolé", "Désolée"},
		{"inquiet", "inquiète"}, {"Inquiet", "Inquiète"},
		{"seul", "seule"}, {"Seul", "Seule"},
		{"certain", "certaine"}, {"Certain", "Certaine"},
		{"plein", "pleine"}, {"Plein", "Pleine"},
		{"fier", "fière"}, {"Fier", "Fière"},
		{"dernier", "dernière"}, {"Dernier", "Dernière"},
		{"premier", "première"}, {"Premier", "Première"},
	}

	allRules["fr:m"] = []Rule{
		{"fatiguée", "fatigué"}, {"Fatiguée", "Fatigué"},
		{"désolée", "désolé"}, {"Désolée", "Désolé"},
		{"heureuse", "heureux"}, {"Heureuse", "Heureux"},
		{"seule", "seul"}, {"Seule", "Seul"},
	}
}
