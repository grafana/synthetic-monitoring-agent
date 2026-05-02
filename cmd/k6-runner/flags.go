package main

import "strings"

// stringList is a flag.Value that parses a comma-separated list. Empty
// items are dropped so the user can write -flag=a,,b without surprises.
type stringList []string

func (s *stringList) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringList) Set(v string) error {
	*s = (*s)[:0]
	for item := range strings.SplitSeq(v, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		*s = append(*s, item)
	}
	return nil
}
