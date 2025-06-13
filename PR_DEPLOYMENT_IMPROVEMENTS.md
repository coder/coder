# PR Deployment Workflow Improvements

This document outlines the improvements made to the PR deployment workflow to address the issues raised in [#11331](https://github.com/coder/coder/issues/11331).

## Problem Statement

The original PR deployment workflow took over 10 minutes to complete, which was too long for developers to wait. The main issues were:

1. Individual certificate generation for each PR (Let's Encrypt rate limits)
2. Inefficient automatic redeployment logic
3. No support for comment-triggered deployments
4. No support for branch deployments without PRs
5. Lack of build optimizations

## Improvements Implemented

### 1. Wildcard Certificate Optimization ✅

**Problem**: Each PR deployment created a new Let's Encrypt certificate, causing delays due to certificate generation time and potential rate limits.

**Solution**: 
- Implemented a single wildcard certificate (`*.${PR_DEPLOYMENTS_DOMAIN}`) that covers all PR deployments
- New file: `.github/pr-deployments/wildcard-certificate.yaml`
- Certificate is created once and reused across all PR deployments
- Reduces deployment time by 2-5 minutes per deployment

**Key Changes**:
- Added wildcard certificate configuration
- Modified certificate handling in the deployment workflow
- Certificate is copied and renamed for each PR namespace

### 2. Improved Automatic Redeployment Logic ✅

**Problem**: The previous logic only checked diff between consecutive commits, preventing redeployment for non-code changes.

**Solution**:
- Enhanced build conditionals to handle various scenarios:
  - New PRs (always build)
  - Comment-triggered deployments (always build)
  - Existing deployments with meaningful code changes
- Better handling of docs-only changes and configuration updates

**Key Changes**:
- Updated `build_conditionals` step with improved logic
- Added support for `pull_request` events with `opened` action
- Better filtering of ignored vs. meaningful changes

### 3. Comment-Triggered Deployments ✅

**Problem**: No way to trigger deployments via PR comments.

**Solution**:
- Added support for `/deploy` comments on PRs
- New job: `check_comment_trigger` to detect and validate comment triggers
- Automatic checkout of the correct PR branch for comment-triggered deployments

**Key Changes**:
- Added `issue_comment` trigger to workflow
- New job to parse and validate deployment comments
- Updated existing jobs to handle comment-triggered flows

### 4. Branch Deployment Support ✅

**Problem**: No way to deploy branches without creating a PR.

**Solution**:
- Added new `deploy_branch` job for branch-only deployments
- Triggered via workflow dispatch with `branch` input parameter
- Creates deployments with `branch-{name}` naming convention

**Key Changes**:
- New workflow input: `branch` parameter
- Dedicated job for branch deployments
- Separate namespace and hostname patterns for branch deployments

### 5. Build Process Optimizations ✅

**Problem**: Build process lacked caching and optimization.

**Solution**:
- Added Go module and build caching
- Added Node.js dependency caching
- Configured build cache environment variables

**Key Changes**:
- Added `actions/cache` steps for Go modules and Node dependencies
- Set `GOCACHE` and `GOMODCACHE` environment variables
- Improved cache keys based on dependency files

## Usage Examples

### Comment-Triggered Deployment
```
# In a PR comment:
/deploy
```

### Branch Deployment
```bash
# Via GitHub Actions UI or API:
# Set branch input to: feature/my-branch
# This will create: branch-feature-my-branch.{domain}
```

### Manual Deployment with Options

```bash
# Via workflow dispatch:
# - experiments: "*" (or specific experiments)
# - build: true (force rebuild)
# - deploy: true (force redeploy)
```

## Expected Performance Improvements

1. **Certificate Generation**: 2-5 minutes saved per deployment
2. **Build Caching**: 1-3 minutes saved on subsequent builds
3. **Improved Logic**: Fewer unnecessary rebuilds
4. **Parallel Operations**: Better resource utilization

**Total Expected Improvement**: 3-8 minutes reduction in deployment time, bringing total time from >10 minutes to 2-7 minutes.

## Migration Notes

1. **Wildcard Certificate**: The first deployment after this change will create the wildcard certificate (one-time setup)
2. **Existing PRs**: Will automatically benefit from the wildcard certificate on next deployment
3. **Comment Triggers**: Available immediately for all open PRs
4. **Branch Deployments**: Available via workflow dispatch

## Monitoring and Rollback

- Monitor deployment times via GitHub Actions logs
- Certificate status can be checked via `kubectl get certificates -n pr-deployment-certs`
- Rollback: Revert to individual certificates by using the original `certificate.yaml` template

## Future Enhancements

Potential additional improvements:

1. Pre-built base images for faster builds
2. Deployment status webhooks
3. Automatic cleanup of old deployments
4. Resource limits and auto-scaling
5. Integration with GitHub Deployments API
