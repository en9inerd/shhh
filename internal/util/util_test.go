package util

import "testing"

func TestStripControl(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"hello\x00world", "helloworld"},
		{"\x1fstart", "start"},
		{"end\x7f", "end"},
		{"mid\x01dle", "middle"},
		{"clean 123", "clean 123"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := StripControl(tc.in); got != tc.want {
			t.Errorf("StripControl(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTruncateUTF8(t *testing.T) {
	cases := []struct {
		in      string
		max     int
		wantLen int // byte length of result
		wantEq  string
	}{
		{"hello", 10, 5, "hello"},
		{"hello", 5, 5, "hello"},
		{"hello", 3, 3, "hel"},
		// Multi-byte: "日" is 3 bytes; truncate at 4 bytes should yield "日" (3 bytes, not 4).
		{"日本語", 4, 3, "日"},
		// Truncate exactly at rune boundary.
		{"日本語", 3, 3, "日"},
		// Empty string.
		{"", 5, 0, ""},
	}
	for _, tc := range cases {
		got := TruncateUTF8(tc.in, tc.max)
		if got != tc.wantEq {
			t.Errorf("TruncateUTF8(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.wantEq)
		}
		if len(got) != tc.wantLen {
			t.Errorf("TruncateUTF8(%q, %d) byte len = %d, want %d", tc.in, tc.max, len(got), tc.wantLen)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{"../../etc/passwd", "../../etc/passwd"}, // path sep not stripped (filepath.Base caller's job)
		{"file\x00name.txt", "filename.txt"},
		{"file\x7fname.txt", "filename.txt"},
		{"normal file (1).txt", "normal file (1).txt"},
	}
	for _, tc := range cases {
		if got := SanitizeFilename(tc.in); got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// 255-byte cap: a 300-byte ASCII filename should be truncated at 255 bytes.
	long := string(make([]byte, 300))
	for i := range long {
		long = long[:i] + "a" + long[i+1:]
	}
	got := SanitizeFilename(long)
	if len(got) > 255 {
		t.Errorf("SanitizeFilename long: len = %d, want ≤ 255", len(got))
	}
}
