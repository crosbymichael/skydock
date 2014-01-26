package utils

import (
	"testing"
)

func TestTruncateName(t *testing.T) {
	var (
		name     = "thisnameis12"
		expected = "thisnameis"
	)

	if actual := Truncate(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}

func TestCleanImageName(t *testing.T) {
	var (
		name     = "crosbymichael/redis:latest"
		expected = "redis"
	)

	if actual := CleanImageImage(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}

func TestCleanImageNameNoParts(t *testing.T) {
	var (
		name     = "redis:latest"
		expected = "redis"
	)

	if actual := CleanImageImage(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}
