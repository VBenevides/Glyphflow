# Release checklist

Release artifacts must include SPDX dependency data, an SBOM, checksums, and
detached signatures. Builds use pinned Go and Node versions and the targets in
`internal/PLATFORMS.md`. The release process is:

1. Run backend tests and frontend build.
2. Build Linux amd64, Linux arm64, and Windows amd64 workers.
3. Generate and sign checksums and the SBOM.
4. Verify signatures and inspect the artifact contents for private keys.
5. Publish the exact tool versions and commands used.
