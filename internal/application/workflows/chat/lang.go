package chat

func detectLang(text string) string {
	hasThai := false
	hasCJK := false
	for _, r := range text {
		switch {
		case r >= 0x0E00 && r <= 0x0E7F:
			hasThai = true
		case (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3040 && r <= 0x30FF):
			hasCJK = true
		}
	}
	if hasThai {
		return "th"
	}
	if hasCJK {
		return "cjk"
	}
	return "en"
}
