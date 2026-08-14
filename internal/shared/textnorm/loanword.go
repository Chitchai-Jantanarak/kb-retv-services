package textnorm

import "strings"

var loanwordReplacer = strings.NewReplacer(
	"รีพอร์ต", " report ",
	"รีพอร์ท", " report ",
	"รีพอรต", " report ",
	"รีพอท", " report ",
	"ดราฟท์", " draft ",
	"ดราฟต์", " draft ",
	"ดร๊าฟ", " draft ",
	"ดราฟ", " draft ",
	"สเตตัส", " status ",
	"ทิคเก็ต", " ticket ",
	"ติกเก็ต", " ticket ",
	"แอสไซน์", " assign ",
)

func NormalizeLoanwords(s string) string {
	replaced := loanwordReplacer.Replace(s)
	if replaced == s {
		return s
	}
	return strings.Join(strings.Fields(replaced), " ")
}
