package resolver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/glebarez/sqlite"
	gitdescribe "github.com/thombashi/gh-git-describe/pkg/executor"
	"gorm.io/gorm"
)

const (
	maxPageSize         = 100
	defaultCacheDirPerm = 0750
)

var shaRegexp = regexp.MustCompile(`^[0-9a-f]{40}$`)

// IsSHA returns true if the string is valid SHA format
func IsSHA(s string) bool {
	s = strings.TrimSpace(s)
	return shaRegexp.MatchString(s)
}

// ToRepoID returns a repository ID string formatted as "owner/name"
func ToRepoID(repo repository.Repository) string {
	return fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
}

type Resolver struct {
	gqlClient  *api.GraphQLClient
	logger     *slog.Logger
	db         *gorm.DB
	cacheTTL   CacheTTL
	gdExecutor gitdescribe.Executor
}

type Params struct {
	// Client is a GraphQL client
	Client *api.GraphQLClient

	// GitDescExecutor is an executor for the thombashi/gh-git-describe.
	GitDescExecutor gitdescribe.Executor

	// Logger is a Logger used by the resolver
	Logger *slog.Logger

	// CacheDirPath is the path to the cache directory.
	// If not specified, it uses the user cache directory.
	CacheDirPath string

	// CacheDirPerm is the permission for the cache directory.
	// Default is 0750.
	CacheDirPerm os.FileMode

	// ClearCache is a flag to clear the cache.
	// If true, the resolver clears the cache database at the initialization.
	ClearCache bool

	// CacheTTL is the time duration settings for the cache
	CacheTTL CacheTTL

	// LogWithPackage is a flag to add module information to the log.
	LogWithPackage bool
}

// New creates a new resolver
func New(params *Params) (*Resolver, error) {
	if params.Client == nil {
		return nil, errors.New("required a GraphQL client")
	}

	if params.GitDescExecutor == nil {
		return nil, errors.New("required a git-describe executor")
	}

	logger := params.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if params.LogWithPackage {
		logger = logger.With(slog.String("package", "gh-taghash/pkg/resolver"))
	}

	cacheDirPerm := params.CacheDirPerm
	if params.CacheDirPerm == 0 {
		cacheDirPerm = defaultCacheDirPerm
	}

	cacheDirPath, err := makeCacheDir(params.CacheDirPath, cacheDirPerm)
	if err != nil {
		return nil, err
	}

	cacheDBPath := filepath.Join(cacheDirPath, "cache.sqlite3")
	logger.Debug("cache database info", slog.String("path", cacheDBPath), slog.String("ttl", params.CacheTTL.String()))

	db, err := gorm.Open(sqlite.Open(cacheDBPath), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open a database: %w", err)
	}

	if err := db.AutoMigrate(&GitTag{}); err != nil {
		return nil, fmt.Errorf("failed to migrate the database: %w", err)
	}

	if params.ClearCache {
		var deletedCount int64

		logger.Debug("mark as delete all the cache records", slog.String("path", cacheDBPath))
		ctx := context.Background()
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&GitTag{}).Where("1 = 1").Delete(&GitTag{})
			if result.Error != nil {
				return fmt.Errorf("failed to delete records: %w", result.Error)
			}

			deletedCount = result.RowsAffected

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prune cache: %w", err)
		}

		logger.Debug("deleted cache records", slog.Int64("count", deletedCount))
	}

	r := &Resolver{
		gqlClient:  params.Client,
		gdExecutor: params.GitDescExecutor,
		logger:     logger,
		cacheTTL:   params.CacheTTL,
		db:         db,
	}

	return r, nil
}

// FetchTagAndOID fetches tags and OIDs from a GitHub repository
func (r Resolver) FetchTagAndOID(repo repository.Repository) (map[string]string, error) {
	var query struct {
		Repository struct {
			Refs struct {
				Nodes []struct {
					Name   string
					Target struct {
						Oid string
					}
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"refs(refPrefix:\"refs/tags/\", first: $first, after: $after)"`
		} `graphql:"repository(owner:$owner, name:$name)"`
	}

	variables := map[string]interface{}{
		"owner": graphql.String(repo.Owner),
		"name":  graphql.String(repo.Name),
		"first": graphql.Int(maxPageSize),
		"after": graphql.String("null"),
	}
	tagHash := map[string]string{}
	repoID := ToRepoID(repo)

	r.logger.Debug("fetching tags and oids", slog.String("repo", repoID))

	err := r.gqlClient.Query("tag_hash", &query, variables)
	if err != nil {
		return nil, fmt.Errorf("error fetching tag and oid: %w", err)
	}
	for _, node := range query.Repository.Refs.Nodes {
		tagHash[node.Name] = node.Target.Oid
	}

	for query.Repository.Refs.PageInfo.HasNextPage {
		endCursor := query.Repository.Refs.PageInfo.EndCursor
		variables["after"] = graphql.String(endCursor)

		r.logger.Debug("fetching next page tags",
			slog.String("repo", repoID),
			slog.String("cursor", endCursor))

		err := r.gqlClient.Query("tag_hash", &query, variables)
		if err != nil {
			return nil, fmt.Errorf("error fetching tag and oid: error=%w, cursor=%s", err, endCursor)
		}
		for _, node := range query.Repository.Refs.Nodes {
			tagHash[node.Name] = node.Target.Oid
		}
	}

	return tagHash, nil
}

// PruneCache removes expired records from the cache database.
// Records are considered expired if the threshold is later than the expired_at field.
// If the threshold is nil, it uses the current time.
func (r *Resolver) PruneCache(ctx context.Context, threshold *time.Time) error {
	r.logger.Debug("pruning expired records from the cache database")

	if threshold == nil {
		now := time.Now()
		threshold = &now
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&GitTag{}).Where(whereExpired, threshold).Delete(&GitTag{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete expired records: %w", result.Error)
		}

		r.logger.Debug("deleted expired records", slog.Int64("rows", result.RowsAffected))

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to prune cache: %w", err)
	}

	return nil
}

func (r *Resolver) updateCacheDB(ctx context.Context, repo repository.Repository, now *time.Time) error {
	repoID := ToRepoID(repo)

	if now == nil {
		n := time.Now()
		now = &n
	}

	r.logger.Debug("updating the database",
		slog.String("repo", repoID),
		slog.String("time", now.String()),
		slog.String("ttl", r.cacheTTL.String()),
	)

	taghashMap, err := r.FetchTagAndOID(repo)
	if err != nil {
		return fmt.Errorf("failed to fetch tags and oids: %w", err)
	}

	hashToTag := map[string]string{}
	ttlMap := map[string]time.Time{}

	for tag, hash := range taghashMap {
		if existTag, exist := hashToTag[hash]; exist {
			shortTTL := now.Add(r.cacheTTL.GitAliasTagTTL)

			// set a shorter TTL for alias tags because it is more likely to be updated
			ttlMap[tag] = shortTTL
			ttlMap[existTag] = shortTTL
		} else {
			ttlMap[tag] = now.Add(r.cacheTTL.GitTagTTL)
			hashToTag[hash] = tag
		}
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for tag, hash := range taghashMap {
			expiredAt, ok := ttlMap[tag]
			if !ok {
				return fmt.Errorf("failed to get a TTL for the tag: %s", tag)
			}

			taghash := &GitTag{
				RepoID:    repoID,
				Tag:       tag,
				BaseTag:   tag,
				Hash:      hash,
				ExpiredAt: expiredAt,
			}
			if tx.Model(&GitTag{}).Where(&GitTag{RepoID: repoID, Tag: tag, Hash: hash}).Updates(taghash).RowsAffected == 0 {
				result := tx.Model(&GitTag{}).Create(taghash)
				if result.Error != nil {
					return fmt.Errorf("failed to create a record: %w", result.Error)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update the database: %w", err)
	}

	if err := r.PruneCache(ctx, now); err != nil {
		return err
	}

	return nil
}

// ResolveTag resolves a tag to a hash
func (r Resolver) ResolveTag(repo repository.Repository, tag string) (*GitTag, error) {
	return r.ResolveTagContext(context.Background(), repo, tag)
}

// ResolveTagContext resolves a tag to a hash with the specified context
func (r Resolver) ResolveTagContext(ctx context.Context, repo repository.Repository, tag string) (*GitTag, error) {
	if tag == "" {
		return nil, errors.New("require a tag")
	}

	var err error
	var taghash GitTag
	repoID := ToRepoID(repo)
	now := time.Now()

	r.logger.Debug("resolving a tag", slog.String("repo", repoID), slog.String("from", tag))

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&GitTag{RepoID: repoID, Tag: tag}).Where(whereNotExpired, now).First(&taghash)
		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil {
		return &taghash, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record: %w", err)
	}

	if err := r.updateCacheDB(ctx, repo, &now); err != nil {
		return nil, fmt.Errorf("failed to update the cache database: %w", err)
	}

	// retry to fetch the record from the cache database after updating the cache
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&GitTag{RepoID: repoID, Tag: tag}).Where(whereNotExpired, now).First(&taghash)
		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil {
		return &taghash, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record: %w", err)
	}

	hash, err := r.gdExecutor.RunGitRevParseContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to run git-describe: %w", err)
	}

	baseTag, err := r.gdExecutor.RunGitDescribeContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, "--tags", "--abbrev=0", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to run git-describe: %w", err)
	}

	newTagHash := &GitTag{
		RepoID:    repoID,
		Tag:       tag,
		BaseTag:   baseTag,
		Hash:      hash,
		ExpiredAt: now.Add(r.cacheTTL.GitFileTTL),
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Model(&GitTag{}).Where(&GitTag{RepoID: repoID, Tag: tag, Hash: hash}).Updates(newTagHash).RowsAffected == 0 {
			r.logger.Debug("creating a new record", slog.String("tag", tag), slog.String("hash", hash))
			result := tx.Model(&GitTag{}).Create(newTagHash)
			if result.Error != nil {
				return fmt.Errorf("failed to create a record: %w", result.Error)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update the database: %w", err)
	}

	return newTagHash, nil
}

// ResolveHash resolves a commit hash to tags
func (r Resolver) ResolveHash(repo repository.Repository, hash string) ([]GitTag, error) {
	return r.ResolveHashContext(context.Background(), repo, hash)
}

// ResolveHashContext resolves a commit hash to tags with the specified context
func (r Resolver) ResolveHashContext(ctx context.Context, repo repository.Repository, hash string) ([]GitTag, error) {
	if !IsSHA(hash) {
		return nil, fmt.Errorf("invalid SHA: %s", hash)
	}

	var err error
	var taghashes []GitTag
	repoID := ToRepoID(repo)
	now := time.Now()

	r.logger.Debug("resolving a hash", slog.String("repo", repoID), slog.String("from", hash))

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&GitTag{RepoID: repoID, Hash: hash}).Where(whereNotExpired, now).Find(&taghashes)
		if result.Error == nil {
			if len(taghashes) > 0 {
				return nil
			}

			return gorm.ErrRecordNotFound
		}

		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil && len(taghashes) > 0 {
		return taghashes, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record from the cache db: %w", err)
	}

	if err := r.updateCacheDB(ctx, repo, &now); err != nil {
		return nil, err
	}

	// retry to fetch the record from the cache database after updating the cache
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&GitTag{RepoID: repoID, Hash: hash}).Where(whereNotExpired, now).Find(&taghashes)
		if result.Error == nil {
			if len(taghashes) > 0 {
				return nil
			}

			return gorm.ErrRecordNotFound
		}

		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil && len(taghashes) > 0 {
		return taghashes, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record from the cache db: %w", err)
	}

	tag, err := r.gdExecutor.RunGitDescribeContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, "--tags", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to run git-describe: %w", err)
	}

	baseTag, err := r.gdExecutor.RunGitDescribeContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, "--tags", "--abbrev=0", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to run git-describe: %w", err)
	}

	newTagHash := &GitTag{
		RepoID:    repoID,
		Tag:       tag,
		BaseTag:   baseTag,
		Hash:      hash,
		ExpiredAt: now.Add(r.cacheTTL.GitFileTTL),
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Model(&GitTag{}).Where(&GitTag{RepoID: repoID, Tag: tag, Hash: hash}).Updates(newTagHash).RowsAffected == 0 {
			result := tx.Model(&GitTag{}).Create(newTagHash)
			if result.Error != nil {
				return fmt.Errorf("failed to create a record: %w", result.Error)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update the database: %w", err)
	}

	return []GitTag{*newTagHash}, nil
}

// Close closes the resolver
func (r *Resolver) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get a database connection: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close the database connection: %w", err)
	}

	r.db = nil

	return nil
}
