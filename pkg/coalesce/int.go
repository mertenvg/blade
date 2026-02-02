package coalesce

func Int(args ...int) int {
	for _, i := range args {
		if i != 0 {
			return i
		}
	}
	return 0
}
