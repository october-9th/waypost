package search

import "sort"

const hnMinScore = 10

const hnMinKeep = 5

func applyMinScore(hits []Result, min, minKeep int) []Result {
	if min <= 0 || len(hits) == 0 {
		return hits
	}

	kept := make([]Result, 0, len(hits))
	for _, h := range hits {
		if h.Score >= min {
			kept = append(kept, h)
		}
	}
	if len(kept) >= minKeep || minKeep <= 0 {
		return kept
	}

	byScore := make([]Result, len(hits))
	copy(byScore, hits)
	sort.SliceStable(byScore, func(i, j int) bool { return byScore[i].Score > byScore[j].Score })
	if len(byScore) > minKeep {
		byScore = byScore[:minKeep]
	}
	pick := make(map[string]bool, len(byScore))
	for _, r := range byScore {
		pick[r.URL] = true
	}
	out := make([]Result, 0, len(byScore))
	for _, h := range hits {
		if pick[h.URL] {
			out = append(out, h)
		}
	}
	return out
}
