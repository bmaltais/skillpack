# Branch Protection Runbook

**Control:** CM-3 (Configuration Change Control) — ITSG-33 PBMM
**Status:** CI workflow and CODEOWNERS committed; branch protection rules must be applied manually via GitHub UI.

## Why This Document Exists

GitHub branch protection rules are enforced server-side and cannot be committed to the repository. This runbook records the required settings so they can be verified, re-applied after a repo transfer, or audited as part of the SAR package.

## Required Branch Protection Settings for `main`

Navigate to **Settings → Branches → Branch protection rules → Add rule** (or edit the existing `main` rule), set branch name pattern to `main`, and enable the following:

| Setting | Required Value | CM-3 Rationale |
|---------|---------------|----------------|
| Require a pull request before merging | ✅ Enabled | No direct push to main; every change goes through a PR |
| Required approvals | **1** | A second party must review before merge |
| Dismiss stale pull request approvals when new commits are pushed | ✅ Enabled | Re-approval required after each push to the PR branch |
| Require status checks to pass before merging | ✅ Enabled | CI gate must be green before merge |
| Required status checks | `CI / Build & Test` | The job defined in `.github/workflows/ci.yml` |
| Require branches to be up to date before merging | ✅ Enabled | Prevents merging stale branches that bypass CI |
| Do not allow bypassing the above settings | ✅ Enabled | Admins are also subject to the policy |

## CODEOWNERS

`.github/CODEOWNERS` designates `@bmaltais` as the required reviewer for all files (`*`). GitHub enforces a CODEOWNERS review as a required check when branch protection is active.

## Verification

After applying these settings, confirm:

```bash
# Should return "required_pull_request_reviews" and "required_status_checks" blocks
gh api repos/bmaltais/skillpack/branches/main/protection
```

## Change Log

| Date | Applied By | Notes |
|------|-----------|-------|
| *(pending)* | @bmaltais | Initial setup per CM-3 remediation PR #158 |
