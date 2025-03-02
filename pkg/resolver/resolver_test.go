package resolver

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/phsym/console-slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gitdescribe "github.com/thombashi/gh-git-describe/pkg/executor"
)

var testLogger = slog.New(
	console.NewHandler(os.Stderr, &console.HandlerOptions{
		Level: slog.LevelDebug,
	}),
)

func TestIsSHA(t *testing.T) {
	a := assert.New(t)

	testCases := []struct {
		name string
		sha  string
		want bool
	}{
		{
			name: "Valid SHA",
			sha:  "0123456789abcdef0123456789abcdef01234567",
			want: true,
		},
		{
			name: "Valid SHA",
			sha:  "abcdef0123456789abcdef0123456789abcdef01",
			want: true,
		},
		{
			name: "Invalid SHA: too short",
			sha:  "0123456789abcdef0123456789abcdef0123456",
			want: false,
		},
		{
			name: "Invalid SHA: too long",
			sha:  "0123456789abcdef0123456789abcdef012345678",
			want: false,
		},
		{
			name: "Invalid SHA: contains invalid character",
			sha:  "0123456789abcdef0123456789abcdef0123456g",
			want: false,
		},
		{
			name: "Invalid SHA: contains invalid character",
			sha:  "0123456789abcdef0123456789abcdef0123456!",
			want: false,
		},
	}

	for _, tc := range testCases {
		a.Equal(tc.want, IsSHA(tc.sha), tc.name)
	}
}

func TestResolver_ResolveFromTagContext(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	cacheTTL := NewCacheTTL(60 * time.Second)
	gqlClient, err := api.NewGraphQLClient(api.ClientOptions{
		CacheTTL: cacheTTL.QueryTTL,
	})
	r.NoError(err)

	gdExecutor, err := gitdescribe.New(&gitdescribe.Params{
		Logger:         testLogger,
		LogWithPackage: true,
		CacheTTL:       cacheTTL.GitFileTTL,
	})
	r.NoError(err)

	resolver, err := New(&Params{
		Client:          gqlClient,
		GitDescExecutor: gdExecutor,
		Logger:          testLogger,
		CacheDirPath:    t.TempDir(),
		ClearCache:      true,
		CacheTTL:        *cacheTTL,
	})
	r.NoError(err)

	actionsCheckoutRepo := repository.Repository{
		Owner: "actions",
		Name:  "checkout",
	}
	cliCliRepo := repository.Repository{
		Owner: "cli",
		Name:  "cli",
	}
	testCases := []struct {
		repo  repository.Repository
		value string
		want  *GitTag
	}{
		{
			repo:  actionsCheckoutRepo,
			value: "v1.1.0",
			want: &GitTag{
				RepoID:     ToRepoID(actionsCheckoutRepo),
				Tag:        "v1.1.0",
				BaseTag:    "v1.1.0",
				TagHash:    "ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca",
				CommitHash: "0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc",
			},
		},
		{
			repo:  actionsCheckoutRepo,
			value: "v4.1.6-4-g6ccd57f",
			want: &GitTag{
				RepoID:     ToRepoID(actionsCheckoutRepo),
				Tag:        "v4.1.6-4-g6ccd57f",
				BaseTag:    "v4.1.6",
				TagHash:    "6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6",
				CommitHash: "6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6",
			},
		},
		{
			repo:  cliCliRepo,
			value: "v2.8.0",
			want: &GitTag{
				RepoID:     ToRepoID(cliCliRepo),
				Tag:        "v2.8.0",
				BaseTag:    "v2.8.0",
				TagHash:    "eb08a9bd29ef9e8b07815a38a168069caf66f240",
				CommitHash: "eb08a9bd29ef9e8b07815a38a168069caf66f240",
			},
		},
	}
	for _, tc := range testCases {
		for i := 0; i < 2; i++ {
			got, err := resolver.ResolveFromTagContext(context.Background(), tc.repo, tc.value)
			r.NoError(err)
			a.Equal(tc.want.TagHash, got.TagHash, tc.value)
			a.Equal(tc.want.CommitHash, got.CommitHash, tc.value)
			a.Equal(tc.want.RepoID, got.RepoID, tc.repo)
			a.Equal(tc.want.Tag, got.Tag)
			a.Equal(tc.want.BaseTag, got.BaseTag)
		}
	}

	tag := "invalid-tag"
	_, err = resolver.ResolveFromTagContext(context.Background(), actionsCheckoutRepo, tag)
	r.Error(err)

	a.NoError(resolver.Close())
}

func TestResolver_ResolveHashContext(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	cacheTTL := NewCacheTTL(60 * time.Second)
	repo := repository.Repository{
		Owner: "actions",
		Name:  "checkout",
	}
	gqlClient, err := api.NewGraphQLClient(api.ClientOptions{
		CacheTTL: cacheTTL.QueryTTL,
	})
	r.NoError(err)

	gdExecutor, err := gitdescribe.New(&gitdescribe.Params{
		Logger:         testLogger,
		LogWithPackage: true,
		CacheTTL:       cacheTTL.GitFileTTL,
	})
	r.NoError(err)

	resolver, err := New(&Params{
		Client:          gqlClient,
		GitDescExecutor: gdExecutor,
		Logger:          testLogger,
		CacheDirPath:    t.TempDir(),
		ClearCache:      true,
		CacheTTL:        *cacheTTL,
	})
	r.NoError(err)

	testCases := []struct {
		value string
		want  *GitTag
	}{
		{
			value: "ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca",
			want: &GitTag{
				RepoID:     ToRepoID(repo),
				Tag:        "v1.1.0",
				BaseTag:    "v1.1.0",
				TagHash:    "ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca",
				CommitHash: "0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc",
			},
		},
		{
			value: "0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc",
			want: &GitTag{
				RepoID:     ToRepoID(repo),
				Tag:        "v1.1.0",
				BaseTag:    "v1.1.0",
				TagHash:    "ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca",
				CommitHash: "0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc",
			},
		},
		{
			value: "6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6",
			want: &GitTag{
				RepoID:     ToRepoID(repo),
				Tag:        "v4.1.6-4-g6ccd57f",
				BaseTag:    "v4.1.6",
				TagHash:    "6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6",
				CommitHash: "6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6",
			},
		},
	}
	for _, tc := range testCases {
		for i := 0; i < 2; i++ {
			gotTags, err := resolver.ResolveHashContext(context.Background(), repo, tc.value)
			r.NoError(err)
			a.Len(gotTags, 1)

			got := gotTags[0]
			a.Equal(tc.want.TagHash, got.TagHash, tc.value)
			a.Equal(tc.want.CommitHash, got.CommitHash, tc.value)
			a.Equal(tc.want.RepoID, got.RepoID, repo)
			a.Equal(tc.want.Tag, got.Tag)
			a.Equal(tc.want.BaseTag, got.BaseTag)
		}
	}

	sha := "1111111111111111111111111111111111111111"
	_, err = resolver.ResolveHashContext(context.Background(), repo, sha)
	r.Error(err)

	a.NoError(resolver.Close())
}
