package omnichannel

import "testing"

func TestConversationLockKeying(t *testing.T) {
	a := conversationLock(1, 2, "u1")
	b := conversationLock(1, 2, "u1")
	if a != b {
		t.Fatal("same key must map to same mutex")
	}
	if conversationLock(1, 2, "u2") == a && conversationLock(1, 3, "u1") == a && conversationLock(9, 2, "u1") == a {
		t.Fatal("all distinct keys collided onto one shard; hashing broken")
	}
}
