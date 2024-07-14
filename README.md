# gh-taghash

`gh` extension to convert a git commit hash to a git tag name and vice versa on a remote GitHub repository.


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
      --cache-dir string   cache directory path. If not specified, use a user cache directory.
      --cache-ttl string   base cache TTL (time-to-live) (default "48h")
      --log-level string   log level (debug, info, warn, error) (default "info")
      --no-cache           disable cache
  -R, --repo string        GitHub repository ID. If not specified, use the current repository.
      --show-base-tag      show the base tag when resolving a tag from a commit hash
```

### Examples

```
$ gh taghash --repo=actions/checkout v4.1.6
a5ac7e51b41094c92402da3b24376905380afc29
$ gh taghash --repo=actions/checkout a5ac7e51b41094c92402da3b24376905380afc29
v4.1.6
```

```
$ gh taghash --repo=actions/checkout 6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6
v4.1.6-4-g6ccd57f
$ gh taghash --repo=actions/checkout v4.1.6-4-g6ccd57f
6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6
$ gh taghash --repo=actions/checkout 6ccd57f4c5d15bdc2fef309bd9fb6cc9db2ef1c6 --show-base-tag
v4.1.6
```
