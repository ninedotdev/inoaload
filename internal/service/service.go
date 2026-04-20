package service

import (
	"regexp"
	"strings"
)

var regValidName = regexp.MustCompile("[^0-9a-zA-Z]+")

func GetValidName(name string) string {
	return strings.ToLower(regValidName.ReplaceAllString(name, ""))
}
