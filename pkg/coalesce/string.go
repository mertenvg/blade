package coalesce

func String(args ...string) string {
	for _, str := range args {
		if str != "" {
			return str
		}
	}
	return ""
}

func StringPointer(args ...*string) *string {
	for _, s := range args {
		if s != nil && *s != "" {
			return s
		}
	}
	return pointer("")
}
