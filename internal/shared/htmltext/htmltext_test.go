package htmltext

import (
	"strings"
	"testing"
)

func TestStripInlineImagesRemovesImgTag(t *testing.T) {
	in := `<p>หลง</p><img src="data:image/jpeg;base64,/9j/4AAQSkZJRg==">`
	got := StripInlineImages(in)
	if got != "<p>หลง</p>" {
		t.Fatalf("got %q, want %q", got, "<p>หลง</p>")
	}
}

func TestStripInlineImagesRemovesBareDataURL(t *testing.T) {
	in := "before data:image/png;base64,AAAABBBBCCCC after"
	got := StripInlineImages(in)
	if strings.Contains(got, "base64") || strings.Contains(got, "data:") {
		t.Fatalf("data url not stripped: %q", got)
	}
}

func TestStripInlineImagesKeepsOtherMarkup(t *testing.T) {
	in := "<ul><li>a</li><li>b</li></ul>"
	if got := StripInlineImages(in); got != in {
		t.Fatalf("markup changed: got %q want %q", got, in)
	}
}

func TestStripInlineImagesEmpty(t *testing.T) {
	if got := StripInlineImages(""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
