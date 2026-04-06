package grammar

func registerSpanish() {
	allRules["es:f"] = []Rule{
		{"cansado", "cansada"}, {"Cansado", "Cansada"},
		{"contento", "contenta"}, {"Contento", "Contenta"},
		{"preparado", "preparada"}, {"Preparado", "Preparada"},
		{"seguro", "segura"}, {"Seguro", "Segura"},
		{"listo", "lista"}, {"Listo", "Lista"},
		{"enamorado", "enamorada"}, {"Enamorado", "Enamorada"},
		{"enojado", "enojada"}, {"Enojado", "Enojada"},
		{"preocupado", "preocupada"}, {"Preocupado", "Preocupada"},
		{"sorprendido", "sorprendida"}, {"Sorprendido", "Sorprendida"},
		{"agradecido", "agradecida"}, {"Agradecido", "Agradecida"},
		{"emocionado", "emocionada"}, {"Emocionado", "Emocionada"},
		{"interesado", "interesada"}, {"Interesado", "Interesada"},
		{"aburrido", "aburrida"}, {"Aburrido", "Aburrida"},
		{"entusiasmado", "entusiasmada"}, {"Entusiasmado", "Entusiasmada"},
		{"ofendido", "ofendida"}, {"Ofendido", "Ofendida"},
		{"confundido", "confundida"}, {"Confundido", "Confundida"},
		{"asustado", "asustada"}, {"Asustado", "Asustada"},
		{"invitado", "invitada"}, {"Invitado", "Invitada"},
	}

	allRules["es:m"] = []Rule{
		{"cansada", "cansado"}, {"Cansada", "Cansado"},
		{"contenta", "contento"}, {"Contenta", "Contento"},
		{"segura", "seguro"}, {"Segura", "Seguro"},
		{"lista", "listo"}, {"Lista", "Listo"},
		{"enamorada", "enamorado"}, {"Enamorada", "Enamorado"},
	}
}
