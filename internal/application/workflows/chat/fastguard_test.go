package chat

import "testing"

func TestFastGuardOffDomain(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "exact thai smalltalk", text: "กินข้าวหรือยัง", want: true},
		{name: "exact english smalltalk", text: "what did you eat today", want: true},
		{name: "thai travel phrase substring", text: "แนะนำสถานที่ท่องเที่ยวในอังกฤษ", want: true},
		{name: "thai weather phrase substring", text: "ขอพยากรณ์อากาศวันนี้หน่อย", want: true},
		{name: "english travel phrase substring", text: "give me a travel recommendation for London", want: true},
		{name: "english joke phrase substring", text: "please tell me a joke", want: true},
		{name: "support request passes", text: "หุ่นยนต์พัง", want: false},
		{name: "handoff passes", text: "ติดต่อเจ้าหน้าที่", want: false},
		{name: "case status passes", text: "ขอเช็คสถานะเคสของฉัน", want: false},
		{name: "product problem passes", text: "แจ้งปัญหาระบบใช้งานไม่ได้", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fastGuardOffDomain(tt.text); got != tt.want {
				t.Fatalf("fastGuardOffDomain(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
