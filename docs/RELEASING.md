# Quantum Control release process

Quantum Control releases are generated from the repository `VERSION` file.

## Release trigger

A merge to `main` that changes `VERSION`, `CITATION.cff`, this document or the release workflow starts `.github/workflows/release.yml`.

The workflow:

1. validates `VERSION`
2. runs `make check`
3. builds Linux archives for `amd64` and `arm64`
4. includes both `quantum-control` and `qcored` in each archive
5. packages the project legal and notice files
6. writes `SHA256SUMS`
7. creates tag `v<VERSION>` and the matching GitHub Release if absent
8. marks versions containing a hyphen as GitHub pre-releases

The pull-request form of the workflow performs validation and archive builds but cannot publish a Release.

## Zenodo

When the repository is enabled in Zenodo's GitHub integration, the GitHub Release is the archival boundary and can be preserved as a versioned software record with a DOI.

`CITATION.cff` contains stable authorship and project metadata. It intentionally does not declare an SPDX license identifier because Quantum Control uses the custom Starlight Unit Studios Quantum Control Community Source License 1.0. The controlling license text remains `LICENSE.de.md`.

After the first Zenodo archive is created, verify the displayed Zenodo license and replace an automatically selected default with the custom Quantum Control license in Zenodo metadata if necessary.

## Version bump rule

`VERSION` changes only in explicit release-preparation pull requests after the intended milestone is complete and CI is green. A normal development commit must not bump the version merely to force a release.

If a matching GitHub Release already exists, the workflow exits without replacing it.
