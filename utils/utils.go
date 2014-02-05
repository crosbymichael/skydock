package utils

import (
	"strings"
)

func Truncate(name string) string {
	if len(name) > 10 {
		return name[:10]
	}
	return name
}

func checkTag(name string) (bool, int) {
	index := strings.LastIndex(name, ":")
	if index == -1 || strings.Contains(name[index:], "/") {
		return false, -1
	}
	return true, index
}

func RemoveTag(name string) string {
	if hasTag, index := checkTag(name); hasTag {
		return name[:index]
	}
	return name
}

func RemoveSlash(name string) string {
	return strings.Replace(name, "/", "", -1)
}

func SplitURI(uri string) (string, string) {
	arr := strings.Split(uri, "://")
	if len(arr) == 1 {
		return "unix", arr[0]
	}
	prot := arr[0]
	if prot == "http" {
		prot = "tcp"
	}
	return prot, arr[1]
}

func CleanImageName(name string) string {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 1 {
		return RemoveSlash(RemoveTag(name))
	}
	return CleanImageName(parts[1])
}
