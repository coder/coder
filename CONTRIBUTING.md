# Contributing

## Style

Contributions must adhere to [Effective Go](https://go.dev/doc/effective_go). Additional styles should prefer automated checks over documentation; humans are error prone!

### No Unused Packages

Coders write packages that are used during implementation. It's difficult to validate whether an abstraction is valid until it's checked against an implementation. This results in a larger changeset, but provides reviewers with an educated perspective on the contribution.

## Review

> Taken from [Go's review philosophy](https://go.dev/doc/contribute#reviews).

Coders value very thorough reviews. Think of each review comment like a ticket: you are expected to somehow "close" it by acting on it, either by implementing the suggestion or convincing the reviewer otherwise.

After you update the change, go through the review comments and make sure to reply to every one. You can click the "Done" button to reply indicating that you've implemented the reviewer's suggestion; otherwise, click on "Reply" and explain why you have not, or what you have done instead.

It is perfectly normal for changes to go through several round of reviews, with one or more reviewers making new comments every time and then waiting for an updated change before reviewing again. This cycle happens even for experienced contributors, so don't be discouraged by it.

Read [Go's Code Review Comments Wiki](https://github.com/golang/go/wiki/CodeReviewComments) to find common comments made during reviews of Go code.
