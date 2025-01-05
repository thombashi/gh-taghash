package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/pflag"
	"github.com/thombashi/gh-taghash/pkg/resolver"
)

type Flags struct {
	LogLevelStr string
	RepoID      string

	OutputFormat string
	ShowBaseTag  bool

	CacheDirPath string
	CacheTTLStr  string
	NoCache      bool
}

func setFlags() (*Flags, []string, error) {
	var flags Flags
	validOutputFormats := []string{
		"text",
		"json",
	}

	pflag.StringVarP(
		&flags.RepoID,
		"repo",
		"R",
		"",
		"GitHub repository ID. If not specified, use the current repository.",
	)
	pflag.StringVar(
		&flags.LogLevelStr,
		"log-level",
		"info",
		"log level (debug, info, warn, error)",
	)

	pflag.StringVar(
		&flags.OutputFormat,
		"format",
		"text",
		fmt.Sprintf("output format (%s)", strings.Join(validOutputFormats, ", ")),
	)
	pflag.BoolVar(
		&flags.ShowBaseTag,
		"show-base-tag",
		false,
		"show the base tag when resolving a tag from a commit hash",
	)

	pflag.StringVar(
		&flags.CacheDirPath,
		"cache-dir",
		"",
		"cache directory path. If not specified, use a user cache directory.",
	)
	pflag.StringVar(
		&flags.CacheTTLStr,
		"cache-ttl",
		"48h",
		"base cache TTL (time-to-live)",
	)
	pflag.BoolVar(
		&flags.NoCache,
		"no-cache",
		false,
		"disable cache",
	)

	pflag.Parse()

	if flags.RepoID == "" {
		repo, err := repository.Current()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get the current repository: %w", err)
		}

		flags.RepoID = resolver.ToRepoID(repo)
	}

	flags.OutputFormat = strings.ToLower(strings.TrimSpace(flags.OutputFormat))
	if !slices.Contains(validOutputFormats, flags.OutputFormat) {
		return nil, nil, fmt.Errorf("invalid output format (%s), expected one of %s", flags.OutputFormat, strings.Join(validOutputFormats, ", "))
	}

	args := pflag.Args()
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("require at least one tag or hash argument")
	}

	return &flags, args, nil
}
