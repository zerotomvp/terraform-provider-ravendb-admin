package client

import "testing"

func TestNormalizeNotAfter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ravendb-format-7-digit-fractional", "2026-05-05T07:41:19.0000000", "2026-05-05T07:41:19Z"},
		{"ravendb-format-with-microseconds", "2026-05-05T07:41:19.1234567", "2026-05-05T07:41:19Z"},
		{"rfc3339-zulu", "2026-05-05T07:41:19Z", "2026-05-05T07:41:19Z"},
		{"rfc3339-with-offset", "2026-05-05T09:41:19+02:00", "2026-05-05T07:41:19Z"},
		{"rfc3339-nano", "2026-05-05T07:41:19.000000000Z", "2026-05-05T07:41:19Z"},
		{"no-fractional-no-tz", "2026-05-05T07:41:19", "2026-05-05T07:41:19Z"},
		{"unparseable-passthrough", "not-a-timestamp", "not-a-timestamp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeNotAfter(c.in)
			if got != c.want {
				t.Errorf("NormalizeNotAfter(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
