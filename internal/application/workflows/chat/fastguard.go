package chat

import "strings"

var fastGuardSmallTalk = map[string]struct{}{
	"กินข้าวหรือยัง":         {},
	"กินข้าวหรือยังครับ":     {},
	"สบายดีไหม":              {},
	"เหงาจัง":                {},
	"what did you eat today": {},
	"how are you":            {},
	"how are you today":      {},
}

var fastGuardOffTopic = []string{
	"สถานที่ท่องเที่ยว",
	"แนะนำที่เที่ยว",
	"พยากรณ์อากาศ",
	"เล่าเรื่องตลก",
	"เล่นมุกตลก",
	"สูตรอาหาร",
	"ทำอาหาร",
	"ผลบอล",
	"ผลฟุตบอล",
	"เลขเด็ด",
	"หวยงวด",
	"ดูดวง",
	"แนะนำหนัง",
	"travel recommendation",
	"places to visit",
	"tourist attraction",
	"weather forecast",
	"tell me a joke",
	"movie recommendation",
	"football score",
	"lottery number",
	"horoscope",
}

func fastGuardOffDomain(text string) bool {
	norm := strings.ToLower(strings.TrimSpace(text))
	if _, ok := fastGuardSmallTalk[norm]; ok {
		return true
	}
	for _, phrase := range fastGuardOffTopic {
		if strings.Contains(norm, phrase) {
			return true
		}
	}
	return false
}
