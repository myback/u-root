// Copyright 2017-2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Run with `go run checklicenses.go`. This script has one drawback:
// - It does not correct the licenses; it simply outputs a list of files which
//   do not conform and returns 1 if the list is non-empty.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	absPath    = flag.Bool("a", false, "Print absolute paths")
	configFile = flag.String("c", "", "Configuration file in JSON format")
)

type rule struct {
	*regexp.Regexp
	invert bool
}

func accept(s string) rule {
	return rule{
		regexp.MustCompile("^" + s + "$"),
		false,
	}
}

func reject(s string) rule {
	return rule{
		regexp.MustCompile("^" + s + "$"),
		true,
	}
}

// Config contains the rules for license checking.
type Config struct {
	// Licenses is a list of acceptable license headers. Each license is
	// represented by an array of strings, one string per line, without the
	// trailing \n .
	Licenses        [][]string
	licensesRegexps []*regexp.Regexp
	// GoPkg is the Go package name to check for licenses
	GoPkg string
	// Accept is a list of file patterns to include in the license checking
	Accept []string
	accept []rule
	// Reject is a list of file patterns to exclude from the license checking
	Reject []string
	reject []rule
}

// CompileRegexps compiles the regular expressions coming from the JSON
// configuration, and returns an error if an invalid regexp is found.
func (c *Config) CompileRegexps() error {
	for _, licenseRegexps := range c.Licenses {
		licenseRegexp := strings.Join(licenseRegexps, "\n")
		re, err := regexp.Compile(licenseRegexp)
		if err != nil {
			return err
		}
		c.licensesRegexps = append(c.licensesRegexps, re)
	}

	c.accept = make([]rule, 0, len(c.Accept))
	for _, rule := range c.Accept {
		c.accept = append(c.accept, accept(rule))
	}

	c.reject = make([]rule, 0, len(c.Reject))
	for _, rule := range c.Reject {
		c.reject = append(c.reject, reject(rule))
	}

	return nil
}

func main() {
	flag.Parse()

	if *configFile == "" {
		log.Fatal("Config file name cannot be empty")
	}
	buf, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", *configFile, err)
	}
	var config Config
	if err := json.Unmarshal(buf, &config); err != nil {
		log.Fatalf("Cannot unmarshal JSON from config file %s: %v", *configFile, err)
	}
	if err := config.CompileRegexps(); err != nil {
		log.Fatalf("Failed to compile regexps from JSON config: %v", err)
	}

	pkgPath := os.ExpandEnv(config.GoPkg)
	incorrect := []string{}

	// List files added to u-root.
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		log.Fatalln("error running git ls-files:", err)
	}
	files := strings.Fields(string(out))

	rules := append(config.accept, config.reject...)

	// Iterate over files.
outer:
	for _, file := range files {
		// Test rules.
		trimmedPath := strings.TrimPrefix(file, pkgPath)
		for _, r := range rules {
			if r.MatchString(trimmedPath) == r.invert {
				continue outer
			}
		}

		// Make sure it is not a directory.
		info, err := os.Stat(file)
		if err != nil {
			log.Fatalln("cannot stat", file, err)
		}
		if info.IsDir() {
			continue
		}

		// Read from the file.
		r, err := os.Open(file)
		if err != nil {
			log.Fatalln("cannot open", file, err)
		}
		defer r.Close()
		contents, err := ioutil.ReadAll(r)
		if err != nil {
			log.Fatalln("cannot read", file, err)
		}
		var foundone bool
		for _, l := range config.licensesRegexps {
			if l.Match(contents) {
				foundone = true
				break
			}
		}
		if !foundone {
			p := trimmedPath
			if *absPath {
				p = file
			}
			incorrect = append(incorrect, p)
		}
	}
	if err != nil {
		log.Fatal(err)
	}

	// Print files with incorrect licenses.
	if len(incorrect) > 0 {
		fmt.Println(strings.Join(incorrect, "\n"))
		os.Exit(1)
	}
}
