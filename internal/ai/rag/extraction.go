package rag

import (
	"context"
	"strings"
)

type DefaultExtractor struct{}

// troubleshootingTerms covers English and Thai keywords that signal a
// troubleshooting intent. Thai terms use Unicode rune comparison, so they
// survive the strings.ToLower call safely (ToLower is ASCII-only for Thai).
var troubleshootingTerms = map[string]struct{}{
	// English
	"offline": {}, "error": {}, "failed": {}, "fail": {},
	"broken": {}, "not": {}, "cannot": {}, "can't": {},
	"crash": {}, "down": {}, "stuck": {}, "slow": {},
	// Thai
	"ใช้งานไม่ได้": {}, "ไม่ได้": {}, "ล้มเหลว": {}, "ผิดพลาด": {},
	"เสีย": {}, "ขัดข้อง": {}, "ช้า": {}, "ค้าง": {},
	"ติดขัด": {}, "หยุดทำงาน": {}, "แก้ไข": {}, "ปัญหา": {},
}

func (DefaultExtractor) Extract(ctx context.Context, query Query) (Meta, error) {
	if err := ctx.Err(); err != nil {
		return Meta{}, err
	}

	terms := terms(query.Text)
	intent := "general-support"
	for _, term := range terms {
		if _, ok := troubleshootingTerms[term]; ok {
			intent = "troubleshooting"
			break
		}
	}

	return Meta{Intent: intent, Terms: terms}, nil
}

func terms(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, " \t\r\n.,!?;:()[]{}\"'")
		if len([]rune(field)) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}
