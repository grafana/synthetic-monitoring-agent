//go:build ignore

package main

import (
	"bufio"
	"log"
	"os"
	"path"
	"strings"
	"text/template"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("Syntax:\n\t%s {template} {files...}\n", os.Args[0])
	}

	contents, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("E: cannot load template %s: %s", os.Args[1], err)
	}

	tmpl, err := template.New("program").Parse(string(contents))
	if err != nil {
		log.Fatalf("E: cannot parse template %s: %s", os.Args[1], err)
	}

	data := make(map[string]int)

	for _, fn := range os.Args[2:] {
		name, count, err := countActiveSeries(fn)
		if err != nil {
			log.Fatalf("E: cannot count active series in %s: %s", fn, err)
		}

		data[name] = count
	}

	if err := tmpl.Execute(os.Stdout, data); err != nil {
		log.Fatalf("E: cannot process template: %s", err)
	}
}

func countActiveSeries(fn string) (string, int, error) {
	fh, err := os.Open(fn)
	if err != nil {
		return "", 0, err
	}

	defer fh.Close()

	scanner := bufio.NewScanner(fh)

	activeSeries := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		activeSeries++
	}

	return strings.TrimSuffix(path.Base(fn), ".txt"), activeSeries, nil
}
