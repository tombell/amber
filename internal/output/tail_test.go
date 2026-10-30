package output

import (
	"strings"
	"testing"
)

func TestTailRetainsLatestBytes(t *testing.T) {
	for _, chunks := range [][]string{
		{},
		{"small", " output"},
		{strings.Repeat("a", Limit)},
		{strings.Repeat("a", Limit-2), "bcde"},
		{"prefix", strings.Repeat("b", Limit+1), "end"},
		{strings.Repeat("a", Limit), strings.Repeat("b", Limit)},
	} {
		var tail Tail
		for _, chunk := range chunks {
			n, err := tail.Write([]byte(chunk))
			if n != len(chunk) || err != nil {
				t.Fatalf("Write() = %d, %v", n, err)
			}
		}
		want := strings.Join(chunks, "")
		if len(want) > Limit {
			want = "[output truncated]\n" + want[len(want)-Limit:]
		}
		if got := tail.String(); got != want {
			t.Fatalf("retained output differs: got %d bytes, want %d", len(got), len(want))
		}
	}
}
