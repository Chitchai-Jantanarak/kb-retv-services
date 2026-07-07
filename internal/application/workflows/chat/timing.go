package chat

import (
	"time"

	"github.com/my/app/internal/application/dto"
)

func chatTimings(debug bool) map[string]int64 {
	if !debug {
		return nil
	}
	return make(map[string]int64)
}

func timedChat(timings map[string]int64, name string, fn func()) {
	if timings == nil {
		fn()
		return
	}
	start := time.Now()
	defer func() {
		timings[name] = time.Since(start).Milliseconds()
	}()
	fn()
}

func timedChatErr(timings map[string]int64, name string, fn func() error) error {
	if timings == nil {
		return fn()
	}
	start := time.Now()
	defer func() {
		timings[name] = time.Since(start).Milliseconds()
	}()
	return fn()
}

func withChatTimings(resp dto.ChatResponse, timings map[string]int64) dto.ChatResponse {
	if timings != nil {
		resp.StageTimingsMS = timings
	}
	return resp
}
