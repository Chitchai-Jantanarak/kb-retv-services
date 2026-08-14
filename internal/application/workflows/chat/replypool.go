package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
)

type replyPool struct {
	mu       sync.RWMutex
	entries  map[string][]string
	provider ports.LLMProvider
}

func newReplyPool(provider ports.LLMProvider) *replyPool {
	return &replyPool{entries: make(map[string][]string), provider: provider}
}

func poolKey(kind, sub, locale string) string {
	return kind + "/" + sub + "/" + locale
}

func replySeed(companyID int64, lastUser string, messageCount int) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d:%s:%d", companyID, lastUser, messageCount)
	return h.Sum64()
}

func (p *replyPool) pick(kind, sub, locale string, seed uint64) (string, bool) {
	if p == nil {
		return "", false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	variants := p.entries[poolKey(kind, sub, locale)]
	if len(variants) == 0 {
		return "", false
	}
	return variants[seed%uint64(len(variants))], true
}

type replyPoolSpec struct {
	kind   string
	sub    string
	prompt func(locale string) string
}

var replyPoolSpecs = []replyPoolSpec{
	{kind: "social", sub: "greeting", prompt: greetingPoolPrompt},
	{kind: "social", sub: "test", prompt: testPoolPrompt},
	{kind: "social", sub: "thanks", prompt: thanksPoolPrompt},
	{kind: "social", sub: "who", prompt: whoPoolPrompt},
	{kind: "handoff", sub: "", prompt: handoffPoolPrompt},
	{kind: "offtopic", sub: "", prompt: offTopicPoolPrompt},
}

var replyPoolLocales = []string{dto.ChatLocaleThai, dto.ChatLocaleEnglish}

func (p *replyPool) warm(ctx context.Context) {
	if p == nil || p.provider == nil {
		return
	}
	for _, spec := range replyPoolSpecs {
		for _, locale := range replyPoolLocales {
			variants, err := p.generateVariants(ctx, spec.prompt(locale), locale)
			if err != nil || len(variants) == 0 {
				continue
			}
			key := poolKey(spec.kind, spec.sub, locale)
			p.mu.Lock()
			p.entries[key] = variants
			p.mu.Unlock()
		}
	}
}

func (p *replyPool) generateVariants(ctx context.Context, instruction, locale string) ([]string, error) {
	prompt := ports.Prompt{
		System: variantsSystemPrompt(locale),
		User:   instruction,
	}
	completion, err := p.provider.GenerateJSON(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Variants []string `json:"variants"`
	}
	if err := json.Unmarshal([]byte(completion.Text), &parsed); err != nil {
		return nil, err
	}
	variants := make([]string, 0, len(parsed.Variants))
	for _, v := range parsed.Variants {
		if v = strings.TrimSpace(v); v != "" {
			variants = append(variants, v)
		}
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("chat: reply pool: no variants returned")
	}
	return variants, nil
}

func variantsSystemPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return `You write short customer-support chat replies in English. Return ONLY JSON in the shape {"variants": ["...", "...", "...", "...", "...", "..."]} with exactly 6 varied, natural, short replies. No markdown, no extra keys.`
	}
	return `คุณเขียนข้อความตอบกลับสั้น ๆ สำหรับแชทซัพพอร์ตเป็นภาษาไทย ใช้คำลงท้ายสุภาพ "ค่ะ" ตอบกลับเป็น JSON เท่านั้นในรูปแบบ {"variants": ["...", "...", "...", "...", "...", "..."]} จำนวน 6 ข้อความที่หลากหลายและกระชับ ห้ามใส่ markdown หรือคีย์อื่น`
}

func greetingPoolPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Greet the user and briefly mention you can help with support cases, case status, team workload, products, customers, and the knowledge base."
	}
	return "ทักทายผู้ใช้และบอกสั้น ๆ ว่าสามารถช่วยเรื่องเคสบริการ สถานะเคส ภาระงานทีม สินค้า ลูกค้า และคลังความรู้ได้"
}

func testPoolPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Confirm the system is working and invite the user to ask a real question."
	}
	return "ยืนยันว่าระบบทำงานปกติ และชวนให้ผู้ใช้ถามคำถามจริง"
}

func thanksPoolPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Give a brief, warm acknowledgment of the user's thanks."
	}
	return "ตอบขอบคุณกลับสั้น ๆ อย่างสุภาพ"
}

func whoPoolPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Identify yourself as the support assistant and briefly mention you can help with support cases, case status, team workload, products, customers, and the knowledge base."
	}
	return "แนะนำตัวว่าเป็นผู้ช่วยฝ่ายซัพพอร์ต และบอกสั้น ๆ ว่าช่วยเรื่องเคสบริการ สถานะเคส ภาระงานทีม สินค้า ลูกค้า และคลังความรู้ได้"
}

func handoffPoolPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Confirm that a human agent will take over and follow up with the user."
	}
	return "ยืนยันว่ากำลังส่งต่อให้เจ้าหน้าที่ และเจ้าหน้าที่จะติดต่อกลับ"
}

func offTopicPoolPrompt(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Politely say you can only help with support topics, then briefly mention you can help with support cases, case status, team workload, products, customers, and the knowledge base."
	}
	return "บอกอย่างสุภาพว่าช่วยได้เฉพาะเรื่องที่เกี่ยวกับซัพพอร์ต และบอกสั้น ๆ ว่าช่วยเรื่องเคสบริการ สถานะเคส ภาระงานทีม สินค้า ลูกค้า และคลังความรู้ได้"
}
