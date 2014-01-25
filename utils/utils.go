package utils

import (
	"strings"
)

func Truncate(name string) string {
	return name[:10]
}

func RemoveTag(name string) string {
	return strings.Split(name, ":")[0]
}

func RemoveSlash(name string) string {
	return strings.Replace(name, "/", "", -1)
}

func CleanImageImage(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		return RemoveSlash(RemoveTag(name))
	}
	return RemoveSlash(RemoveTag(parts[1]))
}
