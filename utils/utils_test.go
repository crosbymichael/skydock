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

	if actual := CleanImageName(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}

func TestCleanImageNameWithRegistry(t *testing.T) {
	var (
		name     = "registry:5000/crosbymichael/redis:latest"
		expected = "redis"
	)

	if actual := CleanImageName(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}

func TestCleanImageNameNoParts(t *testing.T) {
	var (
		name     = "redis:latest"
		expected = "redis"
	)

	if actual := CleanImageName(name); actual != expected {
		t.Fatalf("Expected %s got %s", expected, actual)
	}
}

func TestSplitURIPathOnly(t *testing.T) {
	var (
		uri           = "/var/run/docker.sock"
		expected_prot = "unix"
		expected_path = "/var/run/docker.sock"
	)

	actual_prot, actual_path := SplitURI(uri);
	if actual_prot != expected_prot {
		t.Fatalf("Expected %s got %s", expected_prot, actual_prot)
	}
	if actual_path != expected_path {
		t.Fatalf("Expected %s got %s", expected_path, actual_path)
	}
}

func TestSplitURIUnix(t *testing.T) {
	var (
		uri           = "unix:///var/run/docker.sock"
		expected_prot = "unix"
		expected_path = "/var/run/docker.sock"
	)

	actual_prot, actual_path := SplitURI(uri);
	if actual_prot != expected_prot {
		t.Fatalf("Expected %s got %s", expected_prot, actual_prot)
	}
	if actual_path != expected_path {
		t.Fatalf("Expected %s got %s", expected_path, actual_path)
	}
}

func TestSplitURITcp(t *testing.T) {
	var (
		uri           = "tcp://172.17.42.1:4243"
		expected_prot = "tcp"
		expected_path = "172.17.42.1:4243"
	)

	actual_prot, actual_path := SplitURI(uri);
	if actual_prot != expected_prot {
		t.Fatalf("Expected %s got %s", expected_prot, actual_prot)
	}
	if actual_path != expected_path {
		t.Fatalf("Expected %s got %s", expected_path, actual_path)
	}
}

func TestSplitURIHttp(t *testing.T) {
	var (
		uri           = "http://172.17.42.1:4243"
		expected_prot = "tcp"
		expected_path = "172.17.42.1:4243"
	)

	actual_prot, actual_path := SplitURI(uri);
	if actual_prot != expected_prot {
		t.Fatalf("Expected %s got %s", expected_prot, actual_prot)
	}
	if actual_path != expected_path {
		t.Fatalf("Expected %s got %s", expected_path, actual_path)
	}
}