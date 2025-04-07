# Integrating with docs-preview-link Workflow

This guide shows how to integrate the `docs-analysis` composite action with the existing `docs-preview-link.yml` workflow, eliminating duplication and consolidating documentation processing.

## Current State

The docs-preview-link.yml workflow currently embeds document analysis functionality directly in the workflow steps, which leads to:
- Code duplication across workflows
- Harder maintenance when metrics need to be updated
- Inconsistent reporting between workflows

## Integration Strategy

We can refactor the `docs-preview-link.yml` workflow to use our new composite action, bringing these benefits:
- Single source of truth for document analysis
- Consistent metrics across all documentation workflows
- Easier maintenance and feature additions
- Improved security and error handling

## Example Integration

Here's how to replace the verify-docs-changes job in the docs-preview-link.yml workflow with our composite action:

```yaml
verify-docs-changes:
  needs: [validate-workflow, delay-start]
  runs-on: ubuntu-latest
  timeout-minutes: 3 # Reduced timeout for verification step
  if: |
    always() && 
    (needs.validate-workflow.result == 'success' || needs.validate-workflow.result == 'skipped')
  permissions:
    contents: read
    pull-requests: read
    checks: write # For creating check runs
    statuses: write # For creating commit statuses
  if: |
    always() && (
      (github.event_name == 'pull_request_target' && 
       (github.event.pull_request.draft == false || contains(github.event.pull_request.labels.*.name, 'run-checks-on-draft'))) ||
      (github.event_name == 'workflow_dispatch') ||
      (github.event_name == 'issue_comment' && github.event.issue.pull_request && 
       (contains(github.event.comment.body, '/docs-preview') || contains(github.event.comment.body, '/docs-help')))
    )
  outputs:
    docs_changed: ${{ steps.docs-analysis.outputs.docs-changed }}
    pr_number: ${{ steps.pr_info.outputs.pr_number }}
    branch_name: ${{ steps.pr_info.outputs.branch_name }}
    repo_owner: ${{ steps.pr_info.outputs.repo_owner }}
    is_fork: ${{ steps.pr_info.outputs.is_fork }}
    is_comment: ${{ steps.pr_info.outputs.is_comment }}
    is_manual: ${{ steps.pr_info.outputs.is_manual }}
    skip: ${{ steps.pr_info.outputs.skip }}
    execution_start_time: ${{ steps.timing.outputs.start_time }}
    has_non_docs_changes: ${{ steps.docs-analysis.outputs.has-non-docs-changes }}
    words_added: ${{ steps.docs-analysis.outputs.words-added }}
    words_removed: ${{ steps.docs-analysis.outputs.words-removed }}
    docs_files_count: ${{ steps.docs-analysis.outputs.docs-files-count }}
    images_added: ${{ steps.docs-analysis.outputs.images-added }}
    manifest_changed: ${{ steps.docs-analysis.outputs.manifest-changed }}
    format_only: ${{ steps.docs-analysis.outputs.format-only }}
  steps:
    # Start timing the execution for performance tracking
    - name: Capture start time
      id: timing
      run: |
        echo "start_time=$(date +%s)" >> $GITHUB_OUTPUT
        echo "::notice::Starting docs preview workflow at $(date)"

    # Apply security hardening to the runner
    - name: Harden Runner
      uses: step-security/harden-runner@latest
      with:
        egress-policy: audit

    - name: Create verification check run
      id: create_check
      uses: actions/github-script@latest
      with:
        github-token: ${{ secrets.GITHUB_TOKEN }}
        script: |
          // [existing script code...]

    - name: Get PR info
      id: pr_info
      run: |
        # [existing script code to get PR number, branch, etc.]

    # Only check out the DEFAULT branch (not the PR code) to verify changes safely
    - name: Check out base repository code
      if: steps.pr_info.outputs.skip != 'true'
      uses: actions/checkout@latest
      with:
        ref: main  # Always use the main branch
        fetch-depth: 5  # Reduce checkout depth for faster runs
        sparse-checkout: |
          ${{ env.DOCS_PRIMARY_PATH }}
          *.md
          README.md
        sparse-checkout-cone-mode: false

    # NEW: Use our composite action instead of duplicate logic
    - name: Analyze documentation changes
      id: docs-analysis
      if: steps.pr_info.outputs.skip != 'true'
      uses: ./.github/actions/docs-analysis
      with:
        docs-path: ${{ env.DOCS_PRIMARY_PATH }}
        pr-ref: ${{ steps.pr_info.outputs.branch_name }}
        base-ref: 'main'
        significant-words-threshold: ${{ env.SIGNIFICANT_WORDS_THRESHOLD }}
        throttle-large-repos: 'true'
        debug-mode: ${{ github.event_name == 'workflow_dispatch' && github.event.inputs.debug == 'true' || 'false' }}

    # Remaining steps can use the outputs from docs-analysis
    - name: Update verification status
      if: github.event_name == 'pull_request_target' || (github.event_name == 'workflow_dispatch' && steps.pr_info.outputs.skip != 'true')
      uses: actions/github-script@latest
      with:
        github-token: ${{ secrets.GITHUB_TOKEN }}
        script: |
          // [script modified to use step.docs-analysis outputs]
```

## Benefits of Integration

1. **Reduced Duplication**: The core document analysis logic is maintained in one place
2. **Consistent Features**: All documentation workflows get the same analysis capabilities
3. **Better Versioning**: Can pin to specific versions of the docs-analysis action
4. **Cleaner Workflow Files**: Simplified workflow YAML with better separation of concerns
5. **Improved Maintenance**: Changes to analysis logic only need to be made in one place
6. **Common Security Model**: Same input validation and security practices across workflows

## Implementation Plan

1. Create a small PR with the composite action (completed)
2. Test the action in isolation on sample PRs
3. Create a new PR that refactors docs-preview-link.yml to use the composite action
4. Refactor any other documentation workflows to use the same action
5. Establish a process for maintaining the shared action