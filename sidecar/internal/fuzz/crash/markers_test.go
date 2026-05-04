package crash

import "testing"

func TestClassify_GoPanic(t *testing.T) {
	excerpt := []string{
		"random log line",
		"panic: runtime error: index out of range [5] with length 3",
		"goroutine 17 [running]:",
		"main.foo(...)",
	}
	c := Classify(excerpt)
	if c.Kind != "go_panic" {
		t.Fatalf("kind = %q, want go_panic", c.Kind)
	}
	if c.MarkerLine != 1 {
		t.Fatalf("marker line = %d, want 1", c.MarkerLine)
	}
}

func TestClassify_RippledAssert(t *testing.T) {
	excerpt := []string{
		"FATAL: assertion failed: foo == bar",
		"...",
	}
	c := Classify(excerpt)
	if c.Kind != "rippled_fatal" {
		t.Fatalf("kind = %q, want rippled_fatal", c.Kind)
	}
}

func TestClassify_Sigsegv(t *testing.T) {
	excerpt := []string{"some segfault: signal SIGSEGV: segmentation violation"}
	c := Classify(excerpt)
	if c.Kind != "sigsegv" {
		t.Fatalf("kind = %q, want sigsegv", c.Kind)
	}
}

func TestClassify_NoMarker(t *testing.T) {
	c := Classify([]string{"benign exit", "goodbye"})
	if c.Kind != "" {
		t.Fatalf("kind = %q, want empty", c.Kind)
	}
}
