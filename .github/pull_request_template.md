<!-- Anything that looks like this is a comment and can't be seen after the Pull Request is created. -->

# Summary

<!-- Summarize your PR between here and the checklist. -->

## Context

<!-- Provide a little reasoning as to why this Pull Request helps and why you have opened it. -->

## Checklist

- [ ] [PR has a meaningful title](https://coder.com/docs/v2/latest/CONTRIBUTING#commit-messages)
  - Use the present tense and imperative mood when describing your changes
- [ ] Summarized changes
- [ ] This PR is ready to merge and is not.
  - If the PR is a work in progress, please add the prefix `WIP:` or `[ WIP ]`
    to the beginning of the title or
    [mark it as draft](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/changing-the-stage-of-a-pull-request#converting-a-pull-request-to-a-draft).
- [ ] **[Breaking changes](https://coder.com/docs/v2/latest/CONTRIBUTING#breaking-changes)**
- [Experimental flags(s) needed](https://coder.com/docs/v2/latest/cli/server#--experiments)
  - [ ] None
  - [ ] Experimental flag name(s): <!-- Experimental feature name(s) here -->
- **User-facing changes**
  - [ ] Not Applicable
  - **OR**
  - [ ] Documentation needed.
    - [ ] Issue filed: <!-- Number/link of that issue here -->
  - **OR**
  - [ ] Documentation included.
    - Follow our styling guide
      https://coder.com/docs/v2/latest/contributing/documentation
- **Testing**
  - [ ] N/A or can only be tested interactively
    - Use `./scripts/deploy-pr.sh` to get a PR deployment or check
      [here](https://coder.com/docs/v2/latest/CONTRIBUTING#deploying-a-pr)
  - **OR**
  - [ ] Make sure you've added a new test if existing tests do not effectively
        test the code changed.
