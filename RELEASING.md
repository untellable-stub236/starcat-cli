# Releasing Starcat CLI

## Repository setup

1. Create the public repositories `starcat-app/starcat-cli` and `starcat-app/homebrew-starcat-cli`.
2. Push both repositories with `main` as the default branch.
3. Add a fine-grained `HOMEBREW_TAP_TOKEN` Actions secret to `starcat-app/starcat-cli`. It only needs Contents read/write permission for `starcat-app/homebrew-starcat-cli`.
4. Enable GitHub Actions and artifact attestations.

## Publish a release

Update `CHANGELOG.md`, commit the release state, and create a semantic-version tag:

```bash
git tag -s v0.1.0 -m "Starcat CLI v0.1.0"
git push origin v0.1.0
```

The Release workflow verifies the code, creates five archives, generates checksums and provenance attestations, publishes the GitHub Release, and updates `Formula/starcat.rb` in the Homebrew tap.

Do not reuse or replace an existing release tag. Publish a new patch version instead.
