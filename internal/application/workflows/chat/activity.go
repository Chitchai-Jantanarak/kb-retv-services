package chat

import "github.com/my/app/internal/application/dto"

func activityLabel(locale, code string) string {
	th := map[string]string{
		"request_checked":    "ตรวจสอบคำขอ",
		"permission_checked": "ตรวจสอบสิทธิ์",
		"searched_knowledge": "ค้นหาแหล่งอ้างอิง",
		"answer_prepared":    "เตรียมคำตอบ",
	}
	en := map[string]string{
		"request_checked":    "Checked request",
		"permission_checked": "Checked permission",
		"searched_knowledge": "Searched references",
		"answer_prepared":    "Prepared answer",
	}
	if locale == dto.ChatLocaleEnglish {
		return en[code]
	}
	return th[code]
}

func activity(locale string, codes ...string) []dto.ChatActivity {
	out := make([]dto.ChatActivity, 0, len(codes))
	for _, code := range codes {
		out = append(out, dto.ChatActivity{Code: code, Label: activityLabel(locale, code)})
	}
	return out
}
