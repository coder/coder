# Coder Security

Coder welcomes feedback from security researchers and the general public to help
improve our security. If you believe you have discovered a vulnerability,
privacy issue, exposed data, or other security issues in any of our assets, we
want to hear from you. This policy outlines steps for reporting vulnerabilities
to us, what we expect, what you can expect from us.

You can see the pretty version [here](https://coder.com/security/policy)

## Why Coder's security matters

If an attacker could fully compromise a Coder installation, they could spin up
expensive workstations, steal valuable credentials, or steal proprietary source
code. We take this risk very seriously and employ routine pen testing,
vulnerability scanning, and code reviews. We also welcome the contributions from
the community that helped make this product possible.

## Where should I report security issues?

Please report security issues to <security@coder.com>, providing all relevant
information. The more details you provide, the easier it will be for us to
triage and fix the issue.

## Out of Scope

Our primary concern is around an abuse of the Coder application that allows an
attacker to gain access to another users workspace, or spin up unwanted
workspaces.

- DOS/DDOS attacks affecting availability --> While we do support rate limiting
  of requests, we primarily leave this to the owner of the Coder installation.
  Our rationale is that a DOS attack only affecting availability is not a
  valuable target for attackers.
- Abuse of a compromised user credential --> If a user credential is compromised
  outside of the Coder ecosystem, then we consider it beyond the scope of our
  application. However, if an unprivileged user could escalate their permissions
  or gain access to another workspace, that is a cause for concern.
- Vulnerabilities in third party systems --> Vulnerabilities discovered in
  out-of-scope systems should be reported to the appropriate vendor or
  applicable authority.

## Our Commitments

When working with us, according to this policy, you can expect us to:

- Respond to your report promptly, and work with you to understand and validate
  your report;
- Strive to keep you informed about the progress of a vulnerability as it is
  processed;
- Work to remediate discovered vulnerabilities in a timely manner, within our
  operational constraints; and
- Extend Safe Harbor for your vulnerability research that is related to this
  policy.

## Our Expectations

In participating in our vulnerability disclosure program in good faith, we ask
that you:

- Play by the rules, including following this policy and any other relevant
  agreements. If there is any inconsistency between this policy and any other
  applicable terms, the terms of this policy will prevail;
- Report any vulnerability you’ve discovered promptly;
- Avoid violating the privacy of others, disrupting our systems, destroying
  data, and/or harming user experience;
- Use only the Official Channels to discuss vulnerability information with us;
- Provide us a reasonable amount of time (at least 90 days from the initial
  report) to resolve the issue before you disclose it publicly;
- Perform testing only on in-scope systems, and respect systems and activities
  which are out-of-scope;
- If a vulnerability provides unintended access to data: Limit the amount of
  data you access to the minimum required for effectively demonstrating a Proof
  of Concept; and cease testing and submit a report immediately if you encounter
  any user data during testing, such as Personally Identifiable Information
  (PII), Personal Healthcare Information (PHI), credit card data, or proprietary
  information;
- You should only interact with test accounts you own or with explicit
  permission from
- the account holder; and
- Do not engage in extortion.
