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

func TestRemoveTag(t *testing.T) {
	var (
		name     = "crosbymichael/redis:latest"
		expected = "crosbymichael/redis"
	)

	if actual := RemoveTag(name); actual != expected {
		t.Fatalf("Expected %s go %s", expected, actual)
	}
}

func TestRemoveTagWithRegistry(t *testing.T) {
	var (
		name     = "registry:5000/crosbymichael/redis:latest"
		expected = "registry:5000/crosbymichael/redis"
	)

	if actual := RemoveTag(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}

func TestRemoveTagWithRegistryNoTag(t *testing.T) {
	var (
		name     = "registry:5000/crosbymichael/redis"
		expected = "registry:5000/crosbymichael/redis"
	)

	if actual := RemoveTag(name); actual != expected {
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

func TestCleanImageNameWithRegistry(t *testing.T) {
	var (
		name     = "registry:5000/crosbymichael/redis:latest"
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
