package github

import (
	"net/url"
	"regexp"
	"strconv"
)

var issuePathRe = regexp.MustCompile(`^/([^/]+)/([^/]+)/(issues|pull)/(\d+)`)

func NormalizeURL(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() != "github.com" {
		return ""
	}
	m := issuePathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return ""
	}
	return "https://github.com/" + m[1] + "/" + m[2] + "/" + m[3] + "/" + m[4]
}

func RefFromURL(value string) (ref Ref, isPR bool, ok bool) {
	norm := NormalizeURL(value)
	if norm == "" {
		return Ref{}, false, false
	}
	u, _ := url.Parse(norm)
	m := issuePathRe.FindStringSubmatch(u.Path)
	n, _ := strconv.Atoi(m[4])
	return Ref{Owner: m[1], Repo: m[2], Number: n}, m[3] == "pull", true
}
