package omnichannel

import "testing"

func TestLineNormalizeEveryEvent(t *testing.T) {
	raw := []byte(`{"destination":"Udest","events":[
	 {"type":"message","webhookEventId":"e1","deliveryContext":{"isRedelivery":false},"replyToken":"r1","source":{"type":"user","userId":"U1"},"message":{"id":"m1","type":"text","text":"hi"}},
	 {"type":"follow","webhookEventId":"e2","deliveryContext":{"isRedelivery":false},"replyToken":"r2","source":{"type":"user","userId":"U1"}},
	 {"type":"postback","webhookEventId":"e3","deliveryContext":{"isRedelivery":true},"replyToken":"r3","source":{"type":"user","userId":"U1"},"postback":{"data":"{\"intent\":\"open_case\"}"}},
	 {"type":"unfollow","webhookEventId":"e4","source":{"type":"user","userId":"U1"}},
	 {"type":"beacon","webhookEventId":"e5","source":{"type":"user","userId":"U1"}}]}`)
	got, err := LineNormalizer{}.Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 normalized events (beacon skipped), got %d", len(got))
	}
	if got[0].EventType != "message" || got[0].ReplyToken != "r1" || got[0].Request.ExternalMessageID != "m1" {
		t.Errorf("message: %+v", got[0])
	}
	if got[1].EventType != "follow" || got[1].Request.ExternalMessageID != "e2" || got[1].ReplyToken != "r2" {
		t.Errorf("follow: %+v", got[1])
	}
	if got[2].EventType != "postback" || got[2].PostbackData != `{"intent":"open_case"}` || !got[2].IsRedelivery {
		t.Errorf("postback: %+v", got[2])
	}
	if got[3].EventType != "unfollow" {
		t.Errorf("unfollow: %+v", got[3])
	}
	for _, n := range got {
		if n.ExternalSender != "U1" || n.AccountExternalID != "Udest" {
			t.Errorf("sender/account: %+v", n)
		}
	}
}
