package billing

import "testing"

func rate(v float64) *float64 { return &v }

func TestApplyRate(t *testing.T) {
	cases := []struct {
		name  string
		micro int64
		rate  *float64
		want  int64
	}{
		{"nil rate is identity", 12_345, nil, 12_345},
		{"zero stays zero", 0, rate(0.85), 0},
		{"rate 1 is identity", 12_345, rate(1), 12_345},
		{"exact discount", 1_000_000, rate(0.85), 850_000},
		{"rounds up", 1_000_001, rate(0.85), 850_001},
		{"positive input never becomes zero", 1, rate(0.85), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ApplyRate(tc.micro, tc.rate); got != tc.want {
				t.Errorf("ApplyRate(%d, %v) = %d, want %d", tc.micro, tc.rate, got, tc.want)
			}
		})
	}
}
