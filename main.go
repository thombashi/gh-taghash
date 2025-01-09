package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/phsym/console-slog"
	"github.com/thombashi/eoe"
	gitdescribe "github.com/thombashi/gh-git-describe/pkg/executor"
	"github.com/thombashi/gh-taghash/pkg/resolver"
)

const (
	jsonIndent = "    "
)

func newLogger(level slog.Level) *slog.Logger {
	logger := slog.New(
		console.NewHandler(os.Stderr, &console.HandlerOptions{
			Level: level,
		}),
	)

	return logger
}

func printTag(gitTag resolver.GitTag, flags Flags) error {
	switch flags.OutputFormat {
	case "simple", "text":
		if flags.ShowBaseTag {
			fmt.Println(gitTag.BaseTag)
		} else {
			fmt.Println(gitTag.Tag)
		}

	case "json":
		body := map[string]string{
			"tag": gitTag.Tag,
		}

		if flags.ShowBaseTag {
			body["tag"] = gitTag.BaseTag
		}

		jsonData, err := json.MarshalIndent(body, "", jsonIndent)
		if err != nil {
			return fmt.Errorf("failed to marshal a JSON: %w", err)
		}

		fmt.Println(string(jsonData))

	default:
		return fmt.Errorf("unsupported output format: %s", flags.OutputFormat)
	}

	return nil
}

func printHashes(gitTag resolver.GitTag, flags Flags) error {
	const (
		commitHashKey = "commitHash"
		tagHashKey    = "tagHash"
	)

	switch flags.OutputFormat {
	case "simple":
		if gitTag.TagHash == gitTag.CommitHash {
			fmt.Println(gitTag.TagHash)
			return nil
		}

		fmt.Println(gitTag.TagHash)
		fmt.Println(gitTag.CommitHash)

	case "text":
		if gitTag.TagHash == gitTag.CommitHash {
			fmt.Println(gitTag.CommitHash)
			return nil
		}

		fmt.Printf("%s: %s\n", tagHashKey, gitTag.TagHash)
		fmt.Printf("%s: %s\n", commitHashKey, gitTag.CommitHash)

	case "json":
		body := map[string]string{
			tagHashKey:    gitTag.TagHash,
			commitHashKey: gitTag.CommitHash,
		}

		jsonData, err := json.MarshalIndent(body, "", jsonIndent)
		if err != nil {
			return fmt.Errorf("failed to marshal a JSON: %w", err)
		}

		fmt.Println(string(jsonData))

	default:
		return fmt.Errorf("unsupported output format: %s", flags.OutputFormat)
	}

	return nil
}

func main() {
	var err error

	flags, args, err := setFlags()
	eoe.ExitOnError(err, eoe.NewParams().WithMessage("failed to set flags"))

	var logLevel slog.Level
	err = logLevel.UnmarshalText([]byte(flags.LogLevelStr))
	eoe.ExitOnError(err, eoe.NewParams().WithMessage("failed to get a slog level"))

	logger := newLogger(logLevel)
	eoeParams := eoe.NewParams().WithLogger(logger)

	cacheTTL, err := resolver.ParseCacheTTL(flags.CacheTTLStr)
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to parse a cache TTL"))

	if flags.NoCache {
		cacheTTL.QueryTTL = 0
	}

	gqlClient, err := api.NewGraphQLClient(api.ClientOptions{
		CacheTTL: cacheTTL.QueryTTL,
	})
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to create a GitHub client"))

	gdExecutor, err := gitdescribe.New(&gitdescribe.Params{
		Logger:         logger,
		LogWithPackage: true,
		CacheDirPath:   flags.CacheDirPath,
		CacheTTL:       cacheTTL.GitFileTTL,
	})
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to create a git-describe executor"))

	r, err := resolver.New(&resolver.Params{
		Client:          gqlClient,
		GitDescExecutor: gdExecutor,
		Logger:          logger,
		CacheDirPath:    flags.CacheDirPath,
		ClearCache:      flags.NoCache,
		CacheTTL:        *cacheTTL,
		LogWithPackage:  true,
	})
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to create a resolver"))

	repo, err := repository.Parse(flags.RepoID)
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to parse the repository ID"))

	ctx := context.Background()

	for _, arg := range args {
		if resolver.IsSHA(arg) {
			hash := arg
			gitTags, err := r.ResolveHashContext(ctx, repo, hash)
			eoe.ExitOnError(err, eoeParams.WithMessage("failed to resolve a hash"))

			for _, gitTag := range gitTags {
				logger.Debug("resolved a hash", slog.String("from", hash), slog.String("to", gitTag.Tag))
				err = printTag(gitTag, *flags)
				eoe.ExitOnError(err, eoeParams.WithMessage("failed to print a tag"))
			}
		} else {
			gitTag, err := r.ResolveTagContext(ctx, repo, arg)
			eoe.ExitOnError(err, eoeParams.WithMessage("failed to resolve a tag"))

			logger.Debug("resolved a tag", slog.String("from", arg), slog.String("to", gitTag.String()))
			err = printHashes(*gitTag, *flags)
			eoe.ExitOnError(err, eoeParams.WithMessage("failed to print hashes"))
		}
	}
}
