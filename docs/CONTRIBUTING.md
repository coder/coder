# Contributing

## Requirements

Coder requires Go 1.18+, Node 14+, and GNU Make.

### Development workflow

Use the following `make` commands and scripts in development:

- `make dev` runs the frontend and backend development server
- `make build` compiles binaries and release packages
- `make install` installs binaries to `$GOPATH/bin`
- `make test`

## Styling

### Backend

#### Use Go style

Contributions must adhere to the guidelines outlined in [Effective
Go](https://go.dev/doc/effective_go). We prefer linting rules over documenting
styles (run ours with `make lint`); humans are error-prone!

Read [Go's Code Review Comments
Wiki](https://github.com/golang/go/wiki/CodeReviewComments) for information on
common comments made during reviews of Go code.

#### Avoid unused packages

Coder writes packages that are used during implementation. It isn't easy to
validate whether an abstraction is valid until it's checked against an
implementation. This results in a larger changeset, but it provides reviewers
with a holistic perspective regarding the contribution.

### Frontend

#### Follow component conventions

Each component gets its own folder. Make sure you add a test and Storybook
stories for the component as well. By keeping these tidy, the codebase will
remain easy-to-navigate, healthy and maintainable for all contributors.

#### Keep accessibility in mind

We strive to keep our UI accessible. When using colors, avoid adding new
elements with low color contrast. Always use labels on inputs, not just
placeholders. These are important for screen-readers.

## Reviews

> The following information has been borrowed from [Go's review
> philosophy](https://go.dev/doc/contribute#reviews).

Coder values thorough reviews. For each review comment that you receive, please
"close" it by implementing the suggestion or providing an explanation on why the
suggestion isn't the best option. Be sure to do this for each comment; you can
click **Done** to indicate that you've implemented the suggestion, or you can
add a comment explaining why you aren't implementing the suggestion (or what you
chose to implement instead).

It is perfectly normal for changes to go through several rounds of reviews, with
one or more reviewers making new comments every time, then waiting for an
updated change before reviewing again. All contributors, including those from
maintainers, are subject to the same review cycle; this process is not meant to
be applied selectively or to discourage anyone from contributing.
