# Release

Releases are tag-based. A tag like `v0.1.0` creates:

- OS/arch tarballs with the Go binary, built web assets, migrations, deploy
  files, docs, README, license, and Compose file
- A packaged Helm chart
- `checksums.txt`
- A GitHub Release
- A GHCR image tag through the GHCR workflow

## Build Local Artifacts

```sh
VERSION=v0.1.0 make dist
```

Artifacts are written to `dist/`.

Set `PLATFORMS` to limit binary targets:

```sh
VERSION=v0.1.0 PLATFORMS="linux/amd64 linux/arm64" make package
```

## Local GitHub Release

Requires `git`, `gh`, `helm`, `go`, and `npm`.

```sh
VERSION=v0.1.0 make release
```

The script checks for a clean working tree, verifies that the tag does not
already exist locally or remotely, runs `make test` and `make helm-lint`, builds
assets, pushes the annotated tag, and creates the GitHub Release.

Use `SKIP_VERIFY=true` only when checks already ran in CI:

```sh
VERSION=v0.1.0 SKIP_VERIFY=true make release
```

## GitHub Actions Release

Push a tag to run `.github/workflows/release.yml`:

```sh
git tag -a v0.1.0 -m "uvoo-sqviz v0.1.0"
git push origin v0.1.0
```

The workflow runs tests, lints the chart, builds assets, packages the chart, and
publishes the GitHub Release using `GITHUB_TOKEN`.

The GHCR workflow also runs for `v*` tags and publishes semantic image tags such
as `0.1.0` and `0.1`.
