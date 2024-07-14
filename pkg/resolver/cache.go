package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const extensionName = "gh-taghash"

type CacheTTL struct {
	GitAliasTagTTL time.Duration
	GitFileTTL     time.Duration
	GitTagTTL      time.Duration
	QueryTTL       time.Duration
}

func NewCacheTTL(gitTagTTL time.Duration) *CacheTTL {
	queryTTL := gitTagTTL / 2
	gitFileTTL := gitTagTTL * 30

	// set a shorter TTL for alias tags because it is more likely to be updated
	gitAliasTagTTL := gitTagTTL / 8

	return &CacheTTL{
		GitFileTTL:     gitFileTTL,
		GitTagTTL:      gitTagTTL,
		GitAliasTagTTL: gitAliasTagTTL,
		QueryTTL:       queryTTL,
	}
}

func (t CacheTTL) String() string {
	return fmt.Sprintf("{tag-alias=%s, git=%s, tag=%s, query=%s}", t.GitAliasTagTTL, t.GitFileTTL, t.GitTagTTL, t.QueryTTL)
}

// ParseCacheTTL parses a cache TTL string and returns a CacheTTL.
func ParseCacheTTL(ttl string) (*CacheTTL, error) {
	gitTagCacheTTL, err := time.ParseDuration(ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse a cache TTL: %w", err)
	}

	return NewCacheTTL(gitTagCacheTTL), nil
}

func makeCacheDir(dirPath string, dirPerm os.FileMode) (string, error) {
	dirPath = strings.TrimSpace(dirPath)

	if dirPath == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("failed to get the user cache directory: %w", err)
		}

		dirPath = filepath.Join(userCacheDir, extensionName)
	} else {
		dirPath = filepath.Join(dirPath, extensionName)
	}

	dirPath = filepath.Clean(dirPath)

	if err := os.MkdirAll(dirPath, dirPerm); err != nil {
		return "", fmt.Errorf("failed to create a cache directory: %w", err)
	}

	return dirPath, nil
}
