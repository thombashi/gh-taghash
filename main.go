package main

import (
	"context"
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

func newLogger(level slog.Level) *slog.Logger {
	logger := slog.New(
		console.NewHandler(os.Stderr, &console.HandlerOptions{
			Level: level,
		}),
	)

	return logger
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
		Logger:       logger,
		CacheDirPath: flags.CacheDirPath,
		CacheTTL:     cacheTTL.GitFileTTL,
	})
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to create a git-describe executor"))

	r, err := resolver.New(&resolver.Params{
		Client:          gqlClient,
		GitDescExecutor: gdExecutor,
		Logger:          logger,
		CacheDirPath:    flags.CacheDirPath,
		ClearCache:      flags.NoCache,
		CacheTTL:        *cacheTTL,
	})
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to create a resolver"))

	repo, err := repository.Parse(flags.RepoID)
	eoe.ExitOnError(err, eoeParams.WithMessage("failed to parse the repository ID"))

	ctx := context.Background()

	for _, arg := range args {
		if resolver.IsSHA(arg) {
			taghashes, err := r.ResolveHashContext(ctx, repo, arg)
			eoe.ExitOnError(err, eoeParams.WithMessage("failed to resolve a hash"))

			for _, th := range taghashes {
				logger.Debug("resolved a hash", slog.String("from", th.Hash), slog.String("to", th.Tag))
				if flags.ShowBaseTag {
					fmt.Println(th.BaseTag)
				} else {
					fmt.Println(th.Tag)
				}
			}
		} else {
			taghash, err := r.ResolveTagContext(ctx, repo, arg)
			eoe.ExitOnError(err, eoeParams.WithMessage("failed to resolve a tag"))

			logger.Debug("resolved a tag", slog.String("from", arg), slog.String("to", taghash.Hash))
			fmt.Println(taghash.Hash)
		}
	}
}
