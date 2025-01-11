# gh-taghash

`gh` extension to convert a git commit hash to a git tag name and vice versa on a remote GitHub repository.

[![Go Reference](https://pkg.go.dev/badge/github.com/thombashi/gh-taghash.svg)](https://pkg.go.dev/github.com/thombashi/gh-taghash)
[![Go Report Card](https://goreportcard.com/badge/github.com/thombashi/gh-taghash)](https://goreportcard.com/report/github.com/thombashi/gh-taghash)
[![CI](https://github.com/thombashi/gh-taghash/actions/workflows/ci.yaml/badge.svg)](https://github.com/thombashi/gh-taghash/actions/workflows/ci.yaml)
[![CodeQL](https://github.com/thombashi/gh-taghash/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/thombashi/gh-taghash/actions/workflows/github-code-scanning/codeql)


## Installation

```console
gh extension install thombashi/gh-taghash
```


## Upgrade

```console
gh extension upgrade taghash
```


## Usage

### Command help

```
      --cache-dir string       cache directory path. If not specified, use a user cache directory.
      --cache-ttl string       base cache TTL (time-to-live) (default "48h")
      --format string          output format (simple, text, json) (default "simple")
      --log-level string       log level (debug, info, warn, error) (default "info")
      --no-cache               disable cache
  -R, --repo string            GitHub repository ID. If not specified, use the current repository.
      --show-base-tag          show the base tag when resolving a tag from a commit hash
      --sql-log-level string   SQL log level (silent, error, warn, info) (default "warn")
```

### Examples

Converting a git tag to a commit hash:

```
$ gh taghash --repo=actions/checkout v4.1.6
a5ac7e51b41094c92402da3b24376905380afc29
$ gh taghash --repo=actions/checkout a5ac7e51b41094c92402da3b24376905380afc29
v4.1.6
```

Converting a commit hash to a git tag:

```
$ gh taghash --repo=actions/checkout 6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6
v4.1.6-4-g6ccd57f
$ gh taghash --repo=actions/checkout v4.1.6-4-g6ccd57f
6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6
$ gh taghash --repo=actions/checkout 6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6 --show-base-tag
v4.1.6
```

If a git tag contains both tag hash and commit hash information, both will be output:

```
gh taghash --repo=actions/checkout v1.1.0 --format=text
tagHash: ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca
commitHash: 0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc
```

Converting a git tag to hashes in JSON format:

```
$ gh taghash --repo=actions/checkout v1.1.0 --format=json
{
    "commitHash": "0b496e91ec7ae4428c3ed2eeb4c3a40df431f2cc",
    "tagHash": "ec3afacf7f605c9fc12c70bc1c9e1708ddb99eca"
}
```
