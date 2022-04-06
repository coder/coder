# Contributing

## Requirements

`coder` requires Go 1.18+, Node 14+, and GNU Make.

### Development Workflow

The following `make` commands and scripts used in development:

- `make bin` builds binaries
- `make install` installs binaries to `$GOPATH/bin`
- `make test`
- `make release` dry-runs a new release
- `./develop.sh` hot-reloads for frontend development

## Styling

### Go Style

Contributions must adhere to [Effective Go](https://go.dev/doc/effective_go). Linting rules should
be preferred over documenting styles (run ours with `make lint`); humans are error prone!

Read [Go's Code Review Comments Wiki](https://github.com/golang/go/wiki/CodeReviewComments) to find
common comments made during reviews of Go code.

#### No Unused Packages

Coders write packages that are used during implementation. It's difficult to validate whether an
abstraction is valid until it's checked against an implementation. This results in a larger
changeset but provides reviewers with an educated perspective on the contribution.

## Review

> Taken from [Go's review philosophy](https://go.dev/doc/contribute#reviews).

Coders value thorough reviews. Think of each review comment like a ticket: you are expected to
somehow "close" it by acting on it, either by implementing the suggestion or convincing the reviewer
otherwise.

After you update the change, go through the review comments and make sure to reply to every one. You
can click the "Done" button to reply indicating that you've implemented the reviewer's suggestion;
otherwise, click on "Reply" and explain why you have not, or what you have done instead.

It is perfectly normal for changes to go through several round of reviews, with one or more
reviewers making new comments every time and then waiting for an updated change before reviewing
again. All contributors, including experienced maintainers, are subject to the same review cycle;
this process is not meant to be applied selectively or discourage anyone from contribution.
