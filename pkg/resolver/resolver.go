package resolver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
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
	gormlogger "gorm.io/gorm/logger"
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

// NewGormLogger creates a new GORM logger
func NewGormLogger(logLevel gormlogger.LogLevel) gormlogger.Interface {
	return gormlogger.New(
		log.New(os.Stdout, "\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
		})
}

func extractShaFromCommitResourcePath(commitResourcePath string) (string, error) {
	a := strings.Split(commitResourcePath, "/")
	sha := a[len(a)-1]

	if !IsSHA(sha) {
		return "", fmt.Errorf("invalid SHA: %s", sha)
	}

	return sha, nil
}

type Hash struct {
	CommitHash string
	TagHash    string
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

	// GormLogger is a logger for the GORM
	GormLogger gormlogger.Interface

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
		logger = logger.With(slog.String("package", fmt.Sprintf("%s/pkg/resolver", extensionName)))
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

	var gormLogger gormlogger.Interface
	if params.GormLogger != nil {
		gormLogger = params.GormLogger
	} else {
		gormLogger = NewGormLogger(gormlogger.Warn)
	}

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
func (r Resolver) FetchTagAndOID(repo repository.Repository) (map[string]Hash, error) {
	var query struct {
		Repository struct {
			Refs struct {
				Nodes []struct {
					Name   string
					Target struct {
						Oid                string
						CommitResourcePath string
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
	tagHash := map[string]Hash{}
	repoID := ToRepoID(repo)

	r.logger.Debug("fetching tags and oids", slog.String("repo", repoID))

	err := r.gqlClient.Query("tag_hash", &query, variables)
	if err != nil {
		return nil, fmt.Errorf("error fetching tag and oid: %w", err)
	}
	for _, node := range query.Repository.Refs.Nodes {
		sha, err := extractShaFromCommitResourcePath(node.Target.CommitResourcePath)
		if err != nil {
			return nil, err
		}

		tagHash[node.Name] = Hash{
			TagHash:    node.Target.Oid,
			CommitHash: sha,
		}
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
			sha, err := extractShaFromCommitResourcePath(node.Target.CommitResourcePath)
			if err != nil {
				return nil, err
			}

			tagHash[node.Name] = Hash{
				TagHash:    node.Target.Oid,
				CommitHash: sha,
			}
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

	hashToTag := map[Hash]string{}
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

			gitTag := &GitTag{
				RepoID:     repoID,
				Tag:        tag,
				BaseTag:    tag,
				CommitHash: hash.CommitHash,
				TagHash:    hash.TagHash,
				ExpiredAt:  expiredAt,
			}
			where := &GitTag{
				RepoID:     repoID,
				Tag:        tag,
				CommitHash: hash.CommitHash,
				TagHash:    hash.TagHash,
			}
			if tx.Model(&GitTag{}).Where(where).Updates(gitTag).RowsAffected == 0 {
				result := tx.Model(&GitTag{}).Create(gitTag)
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

// ResolveFromTag resolves a tag to a hash
func (r Resolver) ResolveFromTag(repo repository.Repository, tag string) (*GitTag, error) {
	return r.ResolveFromTagContext(context.Background(), repo, tag)
}

func (r Resolver) resolveTagHashFromGitObj(ctx context.Context, repoID, tag string) (string, error) {
	tagHash, err := r.gdExecutor.RunGitRevParseContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, tag)
	if err != nil {
		return "", err
	}

	return tagHash, nil
}

func (r Resolver) resolveCommitHashFromGitObj(ctx context.Context, repoID, tag string) (string, error) {
	commitHash, err := r.gdExecutor.RunGitRevListContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, "-n", "1", tag)
	if err != nil {
		return "", err
	}

	return commitHash, nil
}

func (r Resolver) resolveBaseTagFromGitObj(ctx context.Context, repoID, hash string) (string, error) {
	baseTag, err := r.gdExecutor.RunGitDescribeContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, "--tags", "--abbrev=0", hash)
	if err != nil {
		return "", err
	}

	return baseTag, nil
}

// ResolveFromTagContext resolves a tag to a hash with the specified context
func (r Resolver) ResolveFromTagContext(ctx context.Context, repo repository.Repository, tag string) (*GitTag, error) {
	if tag == "" {
		return nil, errors.New("require a tag")
	}

	var err error
	var gitTag GitTag
	repoID := ToRepoID(repo)
	now := time.Now()

	r.logger.Debug("resolving a tag", slog.String("repo", repoID), slog.String("from", tag))

	// try to fetch the record from the cache database at first
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&GitTag{RepoID: repoID, Tag: tag}).Where(whereNotExpired, now).First(&gitTag)
		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil {
		return &gitTag, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record: %w", err)
	}

	// update the cache database if the record does not exist
	if err := r.updateCacheDB(ctx, repo, &now); err != nil {
		return nil, fmt.Errorf("failed to update the cache database: %w", err)
	}

	// retry to fetch the record from the cache database after updating the cache
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&GitTag{RepoID: repoID, Tag: tag}).Where(whereNotExpired, now).First(&gitTag)
		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil {
		return &gitTag, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record: %w", err)
	}

	// resolve from the git object if the record does not exist

	tagHash, err := r.resolveTagHashFromGitObj(ctx, repoID, tag)
	if err != nil {
		return nil, err
	}

	baseTag, err := r.resolveBaseTagFromGitObj(ctx, repoID, tagHash)
	if err != nil {
		return nil, err
	}

	commitHash, err := r.resolveCommitHashFromGitObj(ctx, repoID, tag)
	if err != nil {
		return nil, err
	}

	newGitTag := &GitTag{
		RepoID:     repoID,
		Tag:        tag,
		BaseTag:    baseTag,
		TagHash:    tagHash,
		CommitHash: commitHash,
		ExpiredAt:  now.Add(r.cacheTTL.GitFileTTL),
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		where := &GitTag{
			RepoID:     repoID,
			Tag:        tag,
			CommitHash: commitHash,
			TagHash:    tagHash,
		}
		if tx.Model(&GitTag{}).Where(where).Updates(newGitTag).RowsAffected == 0 {
			r.logger.Debug("creating a new record", slog.String("tag", where.String()))
			result := tx.Model(&GitTag{}).Create(newGitTag)
			if result.Error != nil {
				return fmt.Errorf("failed to create a record: %w", result.Error)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update the database: %w", err)
	}

	return newGitTag, nil
}

// ResolveFromHash resolves a commit hash to tags
func (r Resolver) ResolveFromHash(repo repository.Repository, hash string) ([]GitTag, error) {
	return r.ResolveFromHashContext(context.Background(), repo, hash)
}

// ResolveFromHashContext resolves a commit hash to tags with the specified context
func (r Resolver) ResolveFromHashContext(ctx context.Context, repo repository.Repository, hash string) ([]GitTag, error) {
	if !IsSHA(hash) {
		return nil, fmt.Errorf("invalid SHA: %s", hash)
	}

	var err error
	var gitTags []GitTag
	repoID := ToRepoID(repo)
	now := time.Now()
	whereTagHash := &GitTag{RepoID: repoID, TagHash: hash}
	whereCommitHash := &GitTag{RepoID: repoID, CommitHash: hash}

	r.logger.Debug("resolving a hash", slog.String("repo", repoID), slog.String("from", hash))

	// try to fetch the record from the cache database at first
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(whereTagHash).Or(whereCommitHash).Where(whereNotExpired, now).Find(&gitTags)
		if result.Error == nil {
			if len(gitTags) > 0 {
				return nil
			}

			return gorm.ErrRecordNotFound
		}

		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil && len(gitTags) > 0 {
		return gitTags, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record from the cache db: %w", err)
	}

	// update the cache database if the record does not exist
	if err := r.updateCacheDB(ctx, repo, &now); err != nil {
		return nil, err
	}

	// retry to fetch the record from the cache database after updating the cache
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(whereTagHash).Or(whereCommitHash).Where(whereNotExpired, now).Find(&gitTags)
		if result.Error == nil {
			if len(gitTags) > 0 {
				return nil
			}

			return gorm.ErrRecordNotFound
		}

		return result.Error
	}, &sql.TxOptions{ReadOnly: true})
	if err == nil && len(gitTags) > 0 {
		return gitTags, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to select record from the cache db: %w", err)
	}

	// resolve from the git object if the record does not exist

	tag, err := r.gdExecutor.RunGitDescribeContext(ctx, &gitdescribe.RepoCloneParams{
		RepoID:   repoID,
		CacheTTL: r.cacheTTL.GitFileTTL,
	}, "--tags", hash)
	if err != nil {
		return nil, err
	}

	baseTag, err := r.resolveBaseTagFromGitObj(ctx, repoID, hash)
	if err != nil {
		return nil, err
	}

	tagHash, err := r.resolveTagHashFromGitObj(ctx, repoID, tag)
	if err != nil {
		return nil, err
	}

	commitHash, err := r.resolveCommitHashFromGitObj(ctx, repoID, tag)
	if err != nil {
		return nil, err
	}

	newGitTag := &GitTag{
		RepoID:     repoID,
		Tag:        tag,
		BaseTag:    baseTag,
		CommitHash: commitHash,
		TagHash:    tagHash,
		ExpiredAt:  now.Add(r.cacheTTL.GitFileTTL),
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		where := &GitTag{
			RepoID:     repoID,
			Tag:        tag,
			CommitHash: commitHash,
			TagHash:    tagHash,
		}
		if tx.Model(&GitTag{}).Where(where).Updates(newGitTag).RowsAffected == 0 {
			result := tx.Model(&GitTag{}).Create(newGitTag)
			if result.Error != nil {
				return fmt.Errorf("failed to create a record: %w", result.Error)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update the database: %w", err)
	}

	return []GitTag{*newGitTag}, nil
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
