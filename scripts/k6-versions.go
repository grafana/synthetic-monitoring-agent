package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

func main() {
	const repo = "grafana/synthetic-monitoring-agent"

	var tags []string
	url := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=100", repo)

	for url != "" {
		resp, err := http.Get(url)
		if err != nil {
			log.Fatalf("making request to dockerhub: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("Docker Hub API returned status: %s", resp.Status)
		}

		var result struct {
			Count   int    `json:"count"`
			Next    string `json:"next"`
			Results []struct {
				Name string `json:"name"`
			}
		}

		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			log.Fatalf("reading response from dockerhub: %v", err)
		}

		for _, tag := range result.Results {
			if !strings.HasPrefix(tag.Name, "v") {
				continue
			}

			if strings.Contains(tag.Name, "browser") {
				continue
			}

			tags = append(tags, tag.Name)
		}

		url = result.Next
	}

	log.Println("Reading additional versions from stdin, ^D to stop reading...")
	scn := bufio.NewScanner(os.Stdin)
	for scn.Scan() {
		tags = append(tags, strings.TrimSpace(scn.Text()))
	}

	if len(tags) == 0 {
		log.Fatalf("No tags found")
	}

	log.Printf("Found %d tags", len(tags))
	slices.SortFunc(tags, shittySemverCompare)
	slices.Reverse(tags)

	log.Println(tags)

	const knownFirstVersionWithK6 = "v0.23.4"
	upTo := slices.IndexFunc(tags, func(v string) bool { return strings.Contains(v, knownFirstVersionWithK6) })
	if upTo == -1 {
		log.Fatalf("First version with k6 %q not found in version list %v", knownFirstVersionWithK6, tags)
	}

	tags = tags[:upTo+1]
	log.Printf("Figuring out k6 version for %d tags after filtering", len(tags))

	var output struct {
		Mapping map[string]string `json:"mapping"`
		Latest  string            `json:"latest"`
		Unknown []string          `json:"unknown"`
	}

	output.Mapping = map[string]string{}
	output.Latest = tags[0]

	for _, tag := range tags {
		log.Printf("Checking %s", tag)

		img := fmt.Sprintf("grafana/synthetic-monitoring-agent:%s", tag)
		k6Version, err := k6VersionInImage(img)
		if err != nil {
			log.Printf("Error figuring out version: %v", err)
			output.Unknown = append(output.Unknown, tag)
			continue
		}

		log.Printf("%s:%s", tag, k6Version)
		output.Mapping[tag] = k6Version
	}

	err := json.NewEncoder(os.Stdout).Encode(output)
	if err != nil {
		log.Fatalf("encoding version map: %v", err)
	}
}

func k6VersionInImage(img string) (string, error) {
	var err error

	for _, entrypoint := range []string{
		"/usr/local/bin/sm-k6",
		"/usr/local/bin/k6",
	} {
		stdout := &bytes.Buffer{}
		cmd := exec.Command("docker", "run", "-i", "--rm", "--entrypoint="+entrypoint, img, "version")
		cmd.Stdout = stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			continue
		}

		return strings.Split(strings.Fields(stdout.String())[1], "-")[0], nil
	}

	return "", err
}

func shittySemverCompare(a, b string) int {
	a, b = strings.Split(a, "-")[0], strings.Split(b, "-")[0]
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			} else {
				return 1
			}
		}
	}
	return 0
}
