package interpolation

import (
	"regexp"
	"strings"
	"text/template"
)

// variableRegex matches a ${variable_name} reference.
//
// The name deliberately does not allow a hyphen. Checks have shipped with literal ${my-var}
// strings in fields that are expanded, and those have always been sent as text, so widening this
// pattern would quietly turn them into variable lookups.
var variableRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// ExpandVariablesToJS turns a string that may contain ${variable} references into a JavaScript
// expression that evaluates to that string, looking each variable up in the script's vars map.
//
// The result is JavaScript source, not the expanded value: "hello" becomes "'hello'" and
// "${v}" becomes "vars['v']". Literal runs are quoted and escaped with template.JSEscape, so a
// value carrying quotes, a backslash or </script> cannot break out of the generated string.
func ExpandVariablesToJS(in string) string {
	if len(in) == 0 {
		return `''`
	}

	var s strings.Builder

	buf := []byte(in)
	locs := variableRegex.FindAllSubmatchIndex(buf, -1)

	p := 0

	for _, loc := range locs {
		if len(loc) < 4 { // put the bounds checker at ease
			panic("unexpected result while expanding variables")
		}

		if s.Len() > 0 {
			s.WriteRune('+')
		}

		if pre := buf[p:loc[0]]; len(pre) > 0 {
			s.WriteRune('\'')
			template.JSEscape(&s, pre)
			s.WriteRune('\'')
			s.WriteRune('+')
		}

		s.WriteString(`vars['`)
		// Because of the capture in the regular expression, the result
		// has two indices that represent the matched substring, and
		// two more indices that represent the capture group.
		s.Write(buf[loc[2]:loc[3]])
		s.WriteString(`']`)

		p = loc[1]
	}

	if len(buf[p:]) > 0 {
		if s.Len() > 0 {
			s.WriteRune('+')
		}

		s.WriteRune('\'')
		template.JSEscape(&s, buf[p:])
		s.WriteRune('\'')
	}

	return s.String()
}

// FindVariableRefs returns every ${variable} reference in the order it appears, duplicates
// included. Each element is the matched text, ${name} and not name, because callers substitute
// the reference itself.
func FindVariableRefs(in string) []string {
	return variableRegex.FindAllString(in, -1)
}
