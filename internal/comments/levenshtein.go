package comments

// Distance returns the Levenshtein edit distance between a and b. It runs in
// O(len(a)*len(b)) time and O(min(len(a), len(b))) space using a rolling row.
func Distance(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			m := ins
			if del < m {
				m = del
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

// SimilarTo reports whether a and b are close enough to be considered the
// same anchor under the fuzzy ladder. The threshold is the maximum allowed
// distance as a fraction of the longer string's length.
func SimilarTo(a, b string, threshold float64) bool {
	if a == b {
		return true
	}
	lenA, lenB := len([]rune(a)), len([]rune(b))
	longest := lenA
	if lenB > longest {
		longest = lenB
	}
	if longest == 0 {
		return false
	}
	allowed := int(float64(longest) * threshold)
	if allowed < 1 {
		allowed = 1
	}
	return Distance(a, b) <= allowed
}
