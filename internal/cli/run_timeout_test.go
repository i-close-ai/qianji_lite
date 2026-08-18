package cli

import "testing"

func TestDefaultRunTimeout(t *testing.T) {
	cases := []struct {
		tier string
		want int
	}{
		{"", timeoutOrdinary},
		{"ordinary", timeoutOrdinary},
		{"strong", timeoutStrong},
		{"strongest", timeoutStrongest},
	}
	for _, tc := range cases {
		got := defaultRunTimeout(tc.tier)
		if got != tc.want {
			t.Fatalf("tier=%q got=%d want=%d", tc.tier, got, tc.want)
		}
	}
}
