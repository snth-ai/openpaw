package grammar

import "strings"

// Gender — грамматический род синта.
type Gender string

const (
	GenderFemale  Gender = "f"
	GenderMale    Gender = "m"
	GenderNeutral Gender = "n"
)

// Rule — одно правило замены (from → to).
type Rule struct {
	From string
	To   string
}

// Guard — пост-процессинг фильтр для грамматического рода.
type Guard struct {
	rules []Rule
}

// New создаёт Guard для конкретного языка и рода.
func New(lang string, gender Gender) *Guard {
	rules := getRules(lang, gender)
	return &Guard{rules: rules}
}

// Fix применяет все правила к тексту.
// Использует case-sensitive замену слов с проверкой границ.
func (g *Guard) Fix(text string) string {
	if len(g.rules) == 0 {
		return text
	}
	for _, r := range g.rules {
		text = replaceWord(text, r.From, r.To)
	}
	return text
}

// replaceWord заменяет целое слово (не подстроку).
// Проверяет что символы вокруг — не буквы.
func replaceWord(text, from, to string) string {
	idx := 0
	for {
		pos := strings.Index(text[idx:], from)
		if pos == -1 {
			break
		}
		pos += idx

		// Проверяем левую границу
		if pos > 0 && isLetter(rune(text[pos-1])) {
			idx = pos + len(from)
			continue
		}

		// Проверяем правую границу
		end := pos + len(from)
		if end < len(text) && isLetter(rune(text[end])) {
			idx = end
			continue
		}

		// Заменяем
		text = text[:pos] + to + text[end:]
		idx = pos + len(to)
	}
	return text
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') ||
		r == 'ё' || r == 'Ё'
}

func getRules(lang string, gender Gender) []Rule {
	key := lang + ":" + string(gender)
	if rules, ok := allRules[key]; ok {
		return rules
	}
	return nil
}

var allRules = map[string][]Rule{}

func init() {
	registerRussian()
	registerSpanish()
	registerFrench()
	registerPortuguese()
}
