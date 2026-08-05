package tools

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	vecmath "github.com/my/app/internal/shared/vec"
)

type bucketProbe struct {
	want string
	text string
}

var bucketAnchors = map[string][]string{
	"capability": {
		"show me the open support cases",
		"what is the status of this ticket",
		"which cases belong to this product",
		"how is work distributed across the team",
		"look up this customer account",
		"ดูรายการเคสที่เปิดอยู่",
		"สถานะของเคสนี้เป็นอย่างไร",
		"サポート案件の一覧を見せて",
		"이 티켓의 상태를 알려줘",
		"покажи открытые заявки",
		"xem danh sách sự cố đang mở",
	},
	"out_of_scope": {
		"order a meal to my address",
		"book a flight ticket for me",
		"write a program that does this",
		"tell me tomorrow's weather",
		"translate this poem into french",
		"สั่งอาหารมาส่งที่บ้าน",
		"จองตั๋วเครื่องบินให้หน่อย",
		"料理を注文して",
		"항공권을 예약해줘",
		"закажи мне еду",
		"đặt đồ ăn giúp tôi",
	},
	"neutral": {
		"hello there",
		"who am i talking to",
		"what can you do",
		"thanks for the help",
		"just checking this works",
		"สวัสดี",
		"คุณคือใคร",
		"こんにちは",
		"안녕하세요",
		"привет",
		"xin chào",
	},
}

var bucketProbes = []bucketProbe{
	{"capability", "cases for Bella Bot"},
	{"capability", "cases for T300"},
	{"capability", "show me the latest cases"},
	{"capability", "who is free right now"},
	{"capability", "what is the status of case REP-1"},
	{"capability", "เคสของ Bella Bot มีอะไรบ้าง"},
	{"capability", "หุ่นยนต์ล้างจานพัง แก้ยังไง"},
	{"capability", "ロボットが壊れた、どうすればいい"},
	{"capability", "지금 누가 시간이 있나요"},
	{"capability", "какие заявки открыты сейчас"},

	{"out_of_scope", "deliver some food to my house tonight"},
	{"out_of_scope", "write me a python script that scrapes a website"},
	{"out_of_scope", "what is the weather forecast tomorrow"},
	{"out_of_scope", "recommend a good comedy movie"},
	{"out_of_scope", "แนะนำหนังตลกเรื่องใหม่หน่อย"},
	{"out_of_scope", "ส่งอาหารให้หน่อยได้ไหมตอนนี้"},
	{"out_of_scope", "明日の天気を教えて"},
	{"out_of_scope", "피자 좀 주문해줘"},
	{"out_of_scope", "напиши мне стихотворение"},

	{"neutral", "test"},
	{"neutral", "hello"},
	{"neutral", "..."},
	{"neutral", "who are you?"},
	{"neutral", "what time is it?"},
	{"neutral", "hi there, are you the support assistant?"},
	{"neutral", "asdfgh"},
	{"neutral", "สวัสดีครับ"},
	{"neutral", "ขอบคุณครับ"},
	{"neutral", "テスト"},
}

func TestProbeBucketSeparation(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Fatal("guard embedder unavailable")
	}
	ctx := context.Background()

	names := make([]string, 0, len(bucketAnchors))
	for name := range bucketAnchors {
		names = append(names, name)
	}
	sort.Strings(names)

	embedded := make(map[string][][]float32, len(names))
	for _, name := range names {
		vecs, err := guard.Embed(ctx, bucketAnchors[name])
		if err != nil {
			t.Fatalf("embed anchors %s: %v", name, err)
		}
		normed := make([][]float32, len(vecs))
		for i, v := range vecs {
			normed[i] = vecmath.Normalize(v)
		}
		embedded[name] = normed
	}

	texts := make([]string, len(bucketProbes))
	for i, p := range bucketProbes {
		texts[i] = p.text
	}
	qvecs, err := guard.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("embed probes: %v", err)
	}

	correct := 0
	marginsRight := []float64{}
	marginsWrong := []float64{}

	t.Logf("%-42s %-13s %-13s %7s %7s", "probe", "want", "got", "top", "margin")
	for i, p := range bucketProbes {
		q := vecmath.Normalize(qvecs[i])
		scores := map[string]float64{}
		for _, name := range names {
			best := -1.0
			for _, a := range embedded[name] {
				if d := vecmath.Dot(q, a); d > best {
					best = d
				}
			}
			scores[name] = best
		}
		type kv struct {
			k string
			v float64
		}
		ranked := make([]kv, 0, len(scores))
		for k, v := range scores {
			ranked = append(ranked, kv{k, v})
		}
		sort.Slice(ranked, func(a, b int) bool { return ranked[a].v > ranked[b].v })

		got := ranked[0].k
		margin := ranked[0].v - ranked[1].v
		flag := ""
		if got == p.want {
			correct++
			marginsRight = append(marginsRight, margin)
		} else {
			flag = "  <-- WRONG"
			marginsWrong = append(marginsWrong, margin)
		}
		t.Logf("%-42s %-13s %-13s %7.3f %7.3f%s", p.text, p.want, got, ranked[0].v, margin, flag)
	}

	sort.Float64s(marginsRight)
	sort.Float64s(marginsWrong)
	t.Logf("")
	t.Logf("bucket accuracy: %d/%d", correct, len(bucketProbes))
	if len(marginsRight) > 0 {
		t.Logf("correct   margins: min=%.3f med=%.3f max=%.3f",
			marginsRight[0], marginsRight[len(marginsRight)/2], marginsRight[len(marginsRight)-1])
	}
	if len(marginsWrong) > 0 {
		t.Logf("incorrect margins: min=%.3f med=%.3f max=%.3f",
			marginsWrong[0], marginsWrong[len(marginsWrong)/2], marginsWrong[len(marginsWrong)-1])
	}
}
