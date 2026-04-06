package memory

import (
	"math"
	"strings"
	"unicode"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25Score вычисляет BM25 score для документа относительно query.
// avgDL — средняя длина документа в коллекции.
// docCount — общее количество документов.
// docFreq — для каждого term: в скольких документах он встречается.
func BM25Score(query, document string, avgDL float64, docCount int, docFreq map[string]int) float64 {
	queryTerms := tokenize(query)
	docTerms := tokenize(document)
	dl := float64(len(docTerms))

	// Term frequency в документе
	tf := make(map[string]int)
	for _, t := range docTerms {
		tf[t]++
	}

	score := 0.0
	for _, term := range queryTerms {
		if tf[term] == 0 {
			continue
		}

		// IDF
		df := docFreq[term]
		if df == 0 {
			df = 1
		}
		idf := math.Log((float64(docCount)-float64(df)+0.5) / (float64(df) + 0.5))
		if idf < 0 {
			idf = 0
		}

		// TF normalization
		tfNorm := (float64(tf[term]) * (bm25K1 + 1)) /
			(float64(tf[term]) + bm25K1*(1-bm25B+bm25B*(dl/avgDL)))

		score += idf * tfNorm
	}

	return score
}

// BuildDocFreq строит document frequency map для коллекции.
func BuildDocFreq(memories []Memory) (map[string]int, float64) {
	docFreq := make(map[string]int)
	totalLen := 0

	for _, m := range memories {
		terms := tokenize(m.Text)
		totalLen += len(terms)

		// Уникальные термы в этом документе
		seen := make(map[string]bool)
		for _, t := range terms {
			if !seen[t] {
				seen[t] = true
				docFreq[t]++
			}
		}
	}

	avgDL := 0.0
	if len(memories) > 0 {
		avgDL = float64(totalLen) / float64(len(memories))
	}

	return docFreq, avgDL
}

// RankBM25 ранжирует память по BM25 score.
func RankBM25(query string, memories []Memory, limit int) []SearchResult {
	if len(memories) == 0 {
		return nil
	}

	docFreq, avgDL := BuildDocFreq(memories)
	docCount := len(memories)

	type scored struct {
		Memory
		score float64
	}

	var results []scored
	for _, m := range memories {
		s := BM25Score(query, m.Text, avgDL, docCount, docFreq)
		if s > 0 {
			results = append(results, scored{m, s})
		}
	}

	// Sort by score desc
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	out := make([]SearchResult, 0, limit)
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, SearchResult{
			Memory:   r.Memory,
			Distance: float32(1.0 / (1.0 + r.score)), // Convert score to distance-like (lower=better)
		})
	}

	return out
}

// tokenize разбивает текст на lowercase tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}
