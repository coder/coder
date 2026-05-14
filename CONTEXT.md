# Coder Authentication Integrations

This context covers how Coder relates user identities to external services for
authentication-backed integrations.

## Language

**External Auth Provider**:
An OAuth-backed service connection that an already authenticated Coder user links to their Coder account.
_Avoid_: SSO provider, primary login method

**First-Class External Auth Provider**:
An external auth provider type whose OAuth defaults and identity mapping behavior are built into Coder.
_Avoid_: Generic OAuth configuration

**Primary Login Method**:
A method a person uses to authenticate to Coder itself.
_Avoid_: External auth provider

**Linear User Mapping**:
The association between a Linear user ID and the Coder user who connected Linear.
_Avoid_: Linear SSO, Linear login

**External User Identity**:
The external provider account identity associated with an external auth connection.
_Avoid_: Coder user, primary login identity

**External User Display Metadata**:
Minimal name, email, and avatar information shown to identify a connected external account.
_Avoid_: Provider profile snapshot, authorization claims

## Relationships

- A **Coder user** may connect zero or more **External Auth Providers**.
- Linear is a **First-Class External Auth Provider**, not just a generic OAuth configuration.
- Linear external auth requests the least privilege needed for identity mapping by default.
- A **Linear User Mapping** belongs to exactly one Coder user and one Linear user ID.
- A **Linear User Mapping** is represented by the external auth link for the Linear provider.
- A **Linear User Mapping** is established when Linear is connected and verified during explicit external auth validation.
- A Linear external auth connection is incomplete without a Linear user ID.
- A **Linear User Mapping** is keyed by external auth provider ID and Linear user ID.
- A **Linear User Mapping** is unique per Linear provider. A Linear user ID maps to at most one Coder user for that provider.
- A **Linear User Mapping** cannot be silently transferred. A connected account mismatch requires reconnection.
- An **External Auth Provider** connection may expose an **External User Identity** that uses the provider's native identifier.
- **External User Display Metadata** contains only login, name, email, and avatar information.
- **External User Display Metadata** may be stored with an **External Auth Provider** connection, but it does not grant authorization.
- Linear external auth tokens are available through the same workspace token surfaces as other **External Auth Providers**.
- Deleting a Coder user removes their **External Auth Provider** connections.
- A **Primary Login Method** authenticates a user to Coder before they can connect an **External Auth Provider**.

## Example dialogue

> **Dev:** "Are we adding Linear as a **Primary Login Method**?"
> **Domain expert:** "No. We are adding Linear as an **External Auth Provider** so integrations can resolve Linear actors to Coder users."

## Flagged ambiguities

- "Log in with Linear" was initially ambiguous. Resolved: this means connecting Linear as an **External Auth Provider**, not authenticating to Coder with Linear.
