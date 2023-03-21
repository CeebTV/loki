// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/tools/doc-generator/main.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/grafana/loki/pkg/loki"
	"github.com/grafana/loki/tools/doc-generator/parse"
)

const (
	maxLineWidth = 80
	tabWidth     = 2
)

func removeFlagPrefix(block *parse.ConfigBlock, prefix string) {
	for _, entry := range block.Entries {
		switch entry.Kind {
		case parse.KindBlock:
			// Skip root blocks
			if !entry.Root {
				removeFlagPrefix(entry.Block, prefix)
			}
		case parse.KindField:
			if strings.HasPrefix(entry.FieldFlag, prefix) {
				entry.FieldFlag = "<prefix>" + entry.FieldFlag[len(prefix):]
			}
		}
	}
}

func annotateFlagPrefix(blocks []*parse.ConfigBlock) {
	// Find duplicated blocks
	groups := map[string][]*parse.ConfigBlock{}
	for _, block := range blocks {
		groups[block.Name] = append(groups[block.Name], block)
	}

	// For each duplicated block, we need to fix the CLI flags, because
	// in the documentation each block will be displayed only once but
	// since they're duplicated they will have a different CLI flag
	// prefix, which we want to correctly document.
	for _, group := range groups {
		if len(group) == 1 {
			continue
		}

		// We need to find the CLI flags prefix of each config block. To do it,
		// we pick the first entry from each config block and then find the
		// different prefix across all of them.
		var flags []string
		for _, block := range group {
			for _, entry := range block.Entries {
				if entry.Kind == parse.KindField {
					if len(entry.FieldFlag) > 0 {
						flags = append(flags, entry.FieldFlag)
					}
					break
				}
			}
		}

		var allPrefixes []string
		for i, prefix := range parse.FindFlagsPrefix(flags) {
			if len(prefix) > 0 {
				group[i].FlagsPrefix = prefix
				allPrefixes = append(allPrefixes, prefix)
			}
		}

		// Store all found prefixes into each block so that when we generate the
		// markdown we also know which are all the prefixes for each root block.
		for _, block := range group {
			block.FlagsPrefixes = allPrefixes
		}
	}

	// Finally, we can remove the CLI flags prefix from the blocks
	// which have one annotated.
	for _, block := range blocks {
		if block.FlagsPrefix != "" {
			removeFlagPrefix(block, block.FlagsPrefix)
		}
	}
}

func generateBlocksMarkdown(blocks []*parse.ConfigBlock) string {
	md := &markdownWriter{}
	md.writeConfigDoc(blocks)
	return md.string()
}

func main() {
	// Parse the generator flags.
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: doc-generator template-file")
		os.Exit(1)
	}

	templatePath := flag.Arg(0)

	// In order to match YAML config fields with CLI flags, we map
	// the memory address of the CLI flag variables and match them with
	// the config struct fields' addresses.
	cfg := &loki.Config{}
	flags := parse.Flags(cfg)

	// Parse the config, mapping each config field with the related CLI flag.
	blocks, err := parse.Config(cfg, flags, parse.RootBlocks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred while generating the doc: %s\n", err.Error())
		os.Exit(1)
	}

	// Annotate the flags prefix for each root block, and remove the
	// prefix wherever encountered in the config blocks.
	annotateFlagPrefix(blocks)

	// Generate documentation markdown.
	data := struct {
		ConfigFile           string
		GeneratedFileWarning string
	}{
		GeneratedFileWarning: "<!-- DO NOT EDIT THIS FILE - This file has been automatically generated from its .template, regenerate with `make doc` from root directory. -->",
		ConfigFile:           generateBlocksMarkdown(blocks),
	}

	// Load the template file.
	tpl := template.New(filepath.Base(templatePath))

	tpl, err = tpl.ParseFiles(templatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred while loading the template %s: %s\n", templatePath, err.Error())
		os.Exit(1)
	}

	// Execute the template to inject generated doc.
	if err := tpl.Execute(os.Stdout, data); err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred while executing the template %s: %s\n", templatePath, err.Error())
		os.Exit(1)
	}
}