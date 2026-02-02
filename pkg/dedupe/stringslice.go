package dedupe

type Empty struct{}

func StringSlice(strs []string) []string {
	seen := make(map[string]Empty)
	result := make([]string, 0, len(strs))
	for _, str := range strs {
		if _, ok := seen[str]; ok {
			continue
		}
		result = append(result, str)
		seen[str] = Empty{}
	}
	return result
}
