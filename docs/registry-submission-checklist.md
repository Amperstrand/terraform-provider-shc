# Terraform Registry Submission Checklist

**Status**: NOT submitted. This document tracks what's needed when ready.

## Prerequisites (done)
- [x] v0.2.0 released with cross-compiled binaries
- [x] GoReleaser config (.goreleaser.yml)
- [x] SHA256 checksums
- [x] terraform-plugin-docs generated (make docs)
- [x] Provider type name: "shc"
- [x] Provider works with terraform init (manual install verified)

## Still needed
- [ ] GPG signing key for release artifacts
- [ ] Namespace registration at registry.terraform.io (sovereignhybridcompute/shc)
- [ ] Signing config in .goreleaser.yml
- [ ] GitHub Actions release workflow that signs + publishes

## Steps when ready
1. Generate GPG key: `gpg --full-generate-key`
2. Add signing config to .goreleaser.yml
3. Register namespace at https://registry.terraform.io/publishers/sign-in
4. Set up GitHub Actions workflow for automated releases
5. Submit first version via the Registry API
6. Verify `terraform init` works with `source = "sovereignhybridcompute/shc"`
