package tools

import (
	"context"
	"encoding/json"
	"fmt"
	vecmath "github.com/my/app/internal/shared/vec"
	"math"
	"os"
	"sort"
	"testing"
	"unicode"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

const offDomainToolID = "__off_domain"

type langProbe struct {
	lang string
	text string
	want string
}

var langProbes = []langProbe{
	{"en", "cases for product Bella Bot", "f5_product_cases"},
	{"en", "show latest cases", "f1_find_cases"},
	{"en", "check status of case REP-1234", "f2_case_status"},
	{"en", "show received emails", "f12_inbound_read"},
	{"en", "create a case from this email", "f13_promote_mail"},
	{"en", "who is available now", "f4_workload"},
	{"en", "test", ""},
	{"en", "tell me a joke", ""},

	{"th", "เคสของสินค้า Bella Bot", "f5_product_cases"},
	{"th", "ขอเคสล่าสุด", "f1_find_cases"},
	{"th", "ตรวจสอบสถานะเคส REP-1234", "f2_case_status"},
	{"th", "ขอดูอีเมลที่ได้รับ", "f12_inbound_read"},
	{"th", "สร้างเคสจากอีเมลนี้", "f13_promote_mail"},
	{"th", "ตอนนี้ใครว่างบ้าง", "f4_workload"},
	{"th", "ทดสอบ", ""},
	{"th", "เล่าเรื่องตลกให้ฟังหน่อย", ""},

	{"ja", "Bella Bot 製品のケース一覧", "f5_product_cases"},
	{"ja", "最新のケースを見せて", "f1_find_cases"},
	{"ja", "ケース REP-1234 のステータスを確認", "f2_case_status"},
	{"ja", "受信したメールを見せて", "f12_inbound_read"},
	{"ja", "このメールからケースを作成して", "f13_promote_mail"},
	{"ja", "今誰が空いていますか", "f4_workload"},
	{"ja", "テスト", ""},
	{"ja", "冗談を言って", ""},

	{"ko", "Bella Bot 제품 케이스 목록", "f5_product_cases"},
	{"ko", "최근 케이스 보여줘", "f1_find_cases"},
	{"ko", "REP-1234 케이스 상태 확인", "f2_case_status"},
	{"ko", "받은 이메일 보여줘", "f12_inbound_read"},
	{"ko", "이 이메일로 케이스 생성해줘", "f13_promote_mail"},
	{"ko", "지금 누가 시간 있어", "f4_workload"},
	{"ko", "테스트", ""},
	{"ko", "농담 하나 해줘", ""},

	{"ru", "заявки по продукту Bella Bot", "f5_product_cases"},
	{"ru", "покажи последние заявки", "f1_find_cases"},
	{"ru", "проверь статус заявки REP-1234", "f2_case_status"},
	{"ru", "покажи полученные письма", "f12_inbound_read"},
	{"ru", "создай заявку из этого письма", "f13_promote_mail"},
	{"ru", "кто сейчас свободен", "f4_workload"},
	{"ru", "тест", ""},
	{"ru", "расскажи анекдот", ""},

	{"zh", "Bella Bot 产品的工单", "f5_product_cases"},
	{"zh", "显示最新工单", "f1_find_cases"},
	{"zh", "查询工单 REP-1234 的状态", "f2_case_status"},
	{"zh", "显示收到的邮件", "f12_inbound_read"},
	{"zh", "从这封邮件创建工单", "f13_promote_mail"},
	{"zh", "现在谁有空", "f4_workload"},
	{"zh", "测试", ""},
	{"zh", "讲个笑话", ""},

	{"vi", "các ca của sản phẩm Bella Bot", "f5_product_cases"},
	{"vi", "hiển thị các ca gần nhất", "f1_find_cases"},
	{"vi", "kiểm tra trạng thái ca REP-1234", "f2_case_status"},
	{"vi", "xem email đã nhận", "f12_inbound_read"},
	{"vi", "tạo ca từ email này", "f13_promote_mail"},
	{"vi", "ai đang rảnh", "f4_workload"},
	{"vi", "kiểm tra", ""},
	{"vi", "kể một câu chuyện cười", ""},

	{"es", "casos del producto Bella Bot", "f5_product_cases"},
	{"es", "muestra los casos más recientes", "f1_find_cases"},
	{"es", "consulta el estado del caso REP-1234", "f2_case_status"},
	{"es", "muestra los correos recibidos", "f12_inbound_read"},
	{"es", "crea un caso desde este correo", "f13_promote_mail"},
	{"es", "quién está disponible ahora", "f4_workload"},
	{"es", "prueba", ""},
	{"es", "cuéntame un chiste", ""},

	{"en", "which cases is Somchai handling", "f3_employee_status"},
	{"th", "สมชายกำลังดูแลเคสไหนอยู่", "f3_employee_status"},
	{"ja", "ソムチャイはどのケースを担当していますか", "f3_employee_status"},
	{"ko", "솜차이는 어떤 케이스를 담당하고 있나요", "f3_employee_status"},
	{"ru", "какими заявками занимается Сомчай", "f3_employee_status"},
	{"zh", "Somchai 正在处理哪些工单", "f3_employee_status"},
	{"vi", "Somchai đang phụ trách những ca nào", "f3_employee_status"},
	{"es", "qué casos está llevando Somchai", "f3_employee_status"},

	{"en", "show the service contract for customer ACME", "f6_customer_service"},
	{"th", "ขอดูสัญญาบริการของลูกค้า ACME", "f6_customer_service"},
	{"ja", "顧客 ACME のサービス契約を見せて", "f6_customer_service"},
	{"ko", "고객 ACME 의 서비스 계약을 보여줘", "f6_customer_service"},
	{"ru", "покажи сервисный контракт клиента ACME", "f6_customer_service"},
	{"zh", "显示客户 ACME 的服务合同", "f6_customer_service"},
	{"vi", "xem hợp đồng dịch vụ của khách hàng ACME", "f6_customer_service"},
	{"es", "muestra el contrato de servicio del cliente ACME", "f6_customer_service"},

	{"en", "how did we solve this error before", "f7_knowledge"},
	{"th", "ปัญหานี้เคยแก้ยังไงมาก่อน", "f7_knowledge"},
	{"ja", "このエラーは以前どうやって解決しましたか", "f7_knowledge"},
	{"ko", "이 오류는 전에 어떻게 해결했었나요", "f7_knowledge"},
	{"ru", "как мы раньше решали эту ошибку", "f7_knowledge"},
	{"zh", "这个错误以前是怎么解决的", "f7_knowledge"},
	{"vi", "lỗi này trước đây đã xử lý thế nào", "f7_knowledge"},
	{"es", "cómo resolvimos este error antes", "f7_knowledge"},

	{"en", "move case REP-1234 to in progress", "f8_update_case"},
	{"th", "เปลี่ยนสถานะเคส REP-1234 เป็นกำลังดำเนินการ", "f8_update_case"},
	{"ja", "ケース REP-1234 を対応中に変更して", "f8_update_case"},
	{"ko", "REP-1234 케이스를 진행 중으로 변경해줘", "f8_update_case"},
	{"ru", "переведи заявку REP-1234 в работу", "f8_update_case"},
	{"zh", "把工单 REP-1234 改为处理中", "f8_update_case"},
	{"vi", "chuyển ca REP-1234 sang đang xử lý", "f8_update_case"},
	{"es", "cambia el caso REP-1234 a en progreso", "f8_update_case"},

	{"en", "give case REP-1234 to Somchai", "f9_assign_case"},
	{"th", "มอบเคส REP-1234 ให้สมชาย", "f9_assign_case"},
	{"ja", "ケース REP-1234 をソムチャイに割り当てて", "f9_assign_case"},
	{"ko", "REP-1234 케이스를 솜차이에게 배정해줘", "f9_assign_case"},
	{"ru", "назначь заявку REP-1234 на Сомчая", "f9_assign_case"},
	{"zh", "把工单 REP-1234 分配给 Somchai", "f9_assign_case"},
	{"vi", "giao ca REP-1234 cho Somchai", "f9_assign_case"},
	{"es", "asigna el caso REP-1234 a Somchai", "f9_assign_case"},

	{"en", "mark case REP-1234 as done", "f10_close_case"},
	{"th", "เคส REP-1234 เสร็จแล้ว ปิดได้เลย", "f10_close_case"},
	{"ja", "ケース REP-1234 を完了として閉じて", "f10_close_case"},
	{"ko", "REP-1234 케이스를 완료 처리해줘", "f10_close_case"},
	{"ru", "закрой заявку REP-1234, всё готово", "f10_close_case"},
	{"zh", "工单 REP-1234 已完成，请关闭", "f10_close_case"},
	{"vi", "đóng ca REP-1234, đã xong rồi", "f10_close_case"},
	{"es", "cierra el caso REP-1234, ya está resuelto", "f10_close_case"},

	{"en", "the robot broke down, open a new ticket", "open_case"},
	{"th", "หุ่นยนต์เสีย ช่วยเปิดเคสใหม่ให้หน่อย", "open_case"},
	{"ja", "ロボットが故障しました、新しいケースを起票して", "open_case"},
	{"ko", "로봇이 고장났어요, 새 케이스를 접수해줘", "open_case"},
	{"ru", "робот сломался, заведи новую заявку", "open_case"},
	{"zh", "机器人坏了，帮我新建一个工单", "open_case"},
	{"vi", "robot bị hỏng, tạo ca mới giúp tôi", "open_case"},
	{"es", "el robot se averió, abre un caso nuevo", "open_case"},
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func asciiOnlyCatalog(catalog []Tool) []Tool {
	out := make([]Tool, 0, len(catalog))
	for _, t := range catalog {
		kept := make([]string, 0, len(t.Anchors))
		for _, a := range t.Anchors {
			if isASCII(a) {
				kept = append(kept, a)
			}
		}
		t.Anchors = kept
		out = append(out, t)
	}
	return out
}

func offDomainAnchors(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("/app/internal/application/intent/anchors.json")
	if err != nil {
		t.Fatalf("read anchors.json: %v", err)
	}
	var byIntent map[string][]string
	if err := json.Unmarshal(raw, &byIntent); err != nil {
		t.Fatalf("parse anchors.json: %v", err)
	}
	return byIntent["off_domain"]
}

func withNegativeTool(catalog []Tool, negatives []string) []Tool {
	out := make([]Tool, len(catalog), len(catalog)+1)
	copy(out, catalog)
	return append(out, Tool{
		ID:      offDomainToolID,
		Intent:  "off_domain",
		Kind:    "read",
		Handler: "noop",
		Anchors: negatives,
	})
}

type scoreRow struct {
	winner   string
	centered float64
	abstain  bool
}

func scoreAgainst(sel *Selector, vec []float32) scoreRow {
	q := vecmath.Normalize(vec)
	type ts struct {
		id string
		s  float64
	}
	all := make([]ts, 0, len(sel.tools))
	sum := 0.0
	for _, tv := range sel.tools {
		best := 0.0
		for _, v := range tv.vecs {
			if d := vecmath.Dot(q, v); d > best {
				best = d
			}
		}
		all = append(all, ts{tv.id, best})
		sum += best
	}
	if len(all) == 0 {
		return scoreRow{}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
	mean := sum / float64(len(all))
	return scoreRow{winner: all[0].id, centered: all[0].s - mean, abstain: all[0].id == offDomainToolID}
}

type langStat struct {
	realTotal   int
	realHit     int
	realMinCent float64
	junkMaxCent float64
	junkCaught  int
	junkTotal   int
	realLeaked  int
}

func newLangStat() *langStat {
	return &langStat{realMinCent: math.Inf(1), junkMaxCent: math.Inf(-1)}
}

func runVariant(t *testing.T, label string, sel *Selector, vecs map[string][]float32) {
	t.Helper()
	order := []string{"en", "th", "ja", "ko", "ru", "zh", "vi", "es"}
	stats := map[string]*langStat{}
	for _, l := range order {
		stats[l] = newLangStat()
	}

	for _, p := range langProbes {
		row := scoreAgainst(sel, vecs[p.text])
		st := stats[p.lang]
		if p.want != "" {
			st.realTotal++
			if row.winner == p.want {
				st.realHit++
			}
			if row.centered < st.realMinCent {
				st.realMinCent = row.centered
			}
			if row.abstain {
				st.realLeaked++
			}
		} else {
			st.junkTotal++
			if row.centered > st.junkMaxCent {
				st.junkMaxCent = row.centered
			}
			if row.abstain {
				st.junkCaught++
			}
		}
	}

	fmt.Printf("\n---- %s ----\n", label)
	fmt.Printf("%-5s %-10s %-12s %-12s %-8s %s\n", "lang", "correct", "realMinCent", "junkMaxCent", "gap", "negTool junk/real")
	totalHit, totalReal, gapOK := 0, 0, 0
	for _, l := range order {
		st := stats[l]
		gap := st.realMinCent - st.junkMaxCent
		if gap > 0 {
			gapOK++
		}
		totalHit += st.realHit
		totalReal += st.realTotal
		fmt.Printf("%-5s %-10s %-12.4f %-12.4f %-8.4f %d/%d %d/%d\n",
			l, fmt.Sprintf("%d/%d", st.realHit, st.realTotal),
			st.realMinCent, st.junkMaxCent, gap,
			st.junkCaught, st.junkTotal, st.realLeaked, st.realTotal)
	}
	fmt.Printf("TOTAL correct %d/%d   languages with positive junk/real gap: %d/8\n",
		totalHit, totalReal, gapOK)
}

func embedAllProbes(ctx context.Context, t *testing.T, emb Embedder) map[string][]float32 {
	t.Helper()
	texts := make([]string, 0, len(langProbes))
	for _, p := range langProbes {
		texts = append(texts, p.text)
	}
	vecs := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += 100 {
		end := min(start+100, len(texts))
		batch, err := emb.Embed(ctx, texts[start:end])
		if err != nil {
			t.Fatalf("embed probes [%d:%d]: %v", start, end, err)
		}
		vecs = append(vecs, batch...)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("embed count %d != %d", len(vecs), len(texts))
	}
	out := make(map[string][]float32, len(texts))
	for i, txt := range texts {
		out[txt] = vecs[i]
	}
	return out
}

func runEmbedder(t *testing.T, label string, emb Embedder, catalog []Tool, negatives []string) {
	t.Helper()
	ctx := context.Background()
	vecs := embedAllProbes(ctx, t, emb)

	selA, err := NewSelector(ctx, emb, catalog)
	if err != nil {
		t.Fatalf("%s selector A: %v", label, err)
	}
	selB, err := NewSelector(ctx, emb, asciiOnlyCatalog(catalog))
	if err != nil {
		t.Fatalf("%s selector B: %v", label, err)
	}
	selC, err := NewSelector(ctx, emb, withNegativeTool(catalog, negatives))
	if err != nil {
		t.Fatalf("%s selector C: %v", label, err)
	}

	fmt.Printf("\n================ %s ================\n", label)
	runVariant(t, "A  catalog anchors as shipped (en+th)", selA, vecs)
	runVariant(t, "B  ascii-only anchors (en)", selB, vecs)
	runVariant(t, "C  catalog + off_domain negative anchors", selC, vecs)
}

func TestCrossLingualAnchorAblation(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	catalog, err := Load(os.DirFS("/app/config/tools"), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	negatives := offDomainAnchors(t)
	fmt.Printf("probes=%d tools=%d off_domain_anchors=%d\n", len(langProbes), len(catalog), len(negatives))

	_, model, shared, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		t.Fatalf("shared embedder: %v", err)
	}
	runEmbedder(t, "SELECTOR "+model, shared, catalog, negatives)

	guard, _, reason, _ := llmboot.GuardEmbedder(cfg, nil)
	fmt.Printf("\nguard: %s\n", reason)
	if guard == nil {
		t.Log("guard embedder unavailable; POTION variants skipped")
		return
	}
	runEmbedder(t, "GUARD potion-multilingual-pca128-int8", guard, catalog, negatives)
}
