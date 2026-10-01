package visual

import "testing"

func TestSamplerDefaults(t *testing.T) {
	s := (Sampler{}).normalized()
	if s.MaxFrames != 48 || s.IntervalUS != 10_000_000 {
		t.Fatalf("defaults: %+v", s)
	}
}
