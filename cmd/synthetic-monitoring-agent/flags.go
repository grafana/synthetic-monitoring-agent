package main

import (
	"flag"
	"strings"
)

type StringList []string

var _ flag.Value = (*StringList)(nil)

func (l *StringList) String() string {
	if l == nil || len(*l) == 0 {
		return ""
	}

	return strings.Join(*l, ", ")
}

func (l *StringList) Set(value string) error {
	for v := range strings.SplitSeq(value, ",") {
		*l = append(*l, strings.TrimSpace(v))
	}

	return nil
}
