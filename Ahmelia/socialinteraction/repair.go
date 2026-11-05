package socialinteraction

type RepairDetector struct{}

func NewRepairDetector() *RepairDetector { return &RepairDetector{} }
func (d *RepairDetector) Detect(input string, previous ResponsePlan) bool {
	return hasAnyLower(input, "no te entendí", "no me entendiste", "eso no", "me refería", "te confundiste")
}
func hasAnyLower(s string, xs ...string) bool {
	for _, x := range xs {
		if containsFold(s, x) {
			return true
		}
	}
	return false
}
func containsFold(s, x string) bool { return indexFold(s, x) >= 0 }
func indexFold(s, x string) int     { return len([]rune(s[:0])) + indexStringFold(s, x) }
func indexStringFold(s, x string) int {
	ls, lx := toLower(s), toLower(x)
	for i := 0; i+len(lx) <= len(ls); i++ {
		if ls[i:i+len(lx)] == lx {
			return i
		}
	}
	return -1
}
func toLower(s string) string {
	r := []rune(s)
	for i, c := range r {
		if c >= 'A' && c <= 'Z' {
			r[i] = c + ('a' - 'A')
		}
	}
	return string(r)
}
