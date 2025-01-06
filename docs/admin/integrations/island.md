# Island Browser Integration

<div>
  <a href="https://github.com/ericpaulsen" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Eric Paulsen</span>
    <img src="https://github.com/ericpaulsen.png" alt="ericpaulsen" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
April 24, 2024

---

[Island](https://www.island.io/) is an enterprise-grade browser, offering a Chromium-based experience
similar to popular web browsers like Chrome and Edge. It includes built-in
security features for corporate applications and data, aiming to bridge the gap
between consumer-focused browsers and the security needs of the enterprise.

Coder natively integrates with Island's feature set, which include data
loss protection (DLP), application awareness, browser session recording, and
single sign-on (SSO). This guide intends to document these feature categories
and how they apply to your Coder deployment.

## General Configuration

### Create an Application Group for Coder

We recommend creating an Application Group specific to Coder in the Island
Management console. This Application Group object will be referenced when
creating browser policies.

[See the Island documentation for creating an Application Group](https://documentation.island.io/docs/create-and-configure-an-application-group-object).

## Advanced Data Loss Protection

Integrate Island's advanced data loss prevention (DLP) capabilities with
Coder's cloud development environment (CDE), enabling you to control the
"last mile" between developers' CDE and their local devices,
ensuring that sensitive IP remains in your centralized environment.

### Block cut, copy, paste, printing, screen share

1. [Create a Data Sandbox Profile](https://documentation.island.io/docs/create-and-configure-a-data-sandbox-profile).

1. Configure the following actions to allow/block (based on your security
   requirements).

   - Screenshot and Screen Share
   - Printing
   - Save Page
   - Clipboard Limitations

1. [Create a Policy Rule](https://documentation.island.io/docs/create-and-configure-a-policy-rule-general) to apply the Data Sandbox Profile.

1. Define the Coder Application group as the Destination Object.

1. Define the Data Sandbox Profile as the Action in the Last Mile Protection
   section.

### Conditionally allow copy on Coder's CLI authentication page

1. [Create a URL Object](https://documentation.island.io/docs/create-and-configure-a-policy-rule-general) with the following configuration.

   - **Include**
   - **URL type**: Wildcard
   - **URL address**: `coder.example.com/cli-auth`
   - **Casing**: Insensitive

1. [Create a Data Sandbox Profile](https://documentation.island.io/docs/create-and-configure-a-data-sandbox-profile).

1. Configure action to allow copy/paste.

1. [Create a Policy Rule](https://documentation.island.io/docs/create-and-configure-a-policy-rule-general) to apply the Data Sandbox Profile.

1. Define the URL Object you created as the Destination Object.

1. Define the Data Sandbox Profile as the Action in the Last Mile Protection
   section.

### Prevent file upload/download from the browser

1. Create a Protection Profiles for both upload/download.

   - [Upload documentation](https://documentation.island.io/docs/create-and-configure-an-upload-protection-profile)
   - [Download documentation](https://documentation.island.io/v1/docs/en/create-and-configure-a-download-protection-profile)

1. [Create a Policy Rule](https://documentation.island.io/docs/create-and-configure-a-policy-rule-general) to apply the Protection Profiles.

1. Define the Coder Application group as the Destination Object.

1. Define the applicable Protection Profile as the Action in the Data Protection
   section.

### Scan files for sensitive data

1. [Create a Data Loss Prevention scanner](https://documentation.island.io/docs/create-a-data-loss-prevention-scanner).

1. [Create a Policy Rule](https://documentation.island.io/docs/create-and-configure-a-policy-rule-general) to apply the DLP Scanner.

1. Define the Coder Application group as the Destination Object.

1. Define the DLP Scanner as the Action in the Data Protection section.

## Application Awareness and Boundaries

Ensure that Coder is only accessed through the Island browser, guaranteeing that
your browser-level DLP policies are always enforced, and developers can't
sidestep such policies simply by using another browser.

### Configure browser enforcement, conditional access policies

Create a conditional access policy for your configured identity provider.

Note that the configured IdP must be the same for both Coder and Island.

- [Azure Active Directory/Entra ID](https://documentation.island.io/docs/configure-browser-enforcement-for-island-with-azure-ad#create-and-apply-a-conditional-access-policy)
- [Okta](https://documentation.island.io/docs/configure-browser-enforcement-for-island-with-okta)
- [Google](https://documentation.island.io/docs/configure-browser-enforcement-for-island-with-google-enterprise)

## Browser Activity Logging

Govern and audit in-browser terminal and IDE sessions using Island, such as
screenshots, mouse clicks, and keystrokes.

### Activity Logging Module

1. [Create an Activity Logging Profile](https://documentation.island.io/docs/create-and-configure-an-activity-logging-profile). Supported browser
   events include:

   - Web Navigation
   - File Download
   - File Upload
   - Clipboard/Drag & Drop
   - Print
   - Save As
   - Screenshots
   - Mouse Clicks
   - Keystrokes

1. [Create a Policy Rule](https://documentation.island.io/docs/create-and-configure-a-policy-rule-general) to apply the Activity Logging Profile.

1. Define the Coder Application group as the Destination Object.

1. Define the Activity Logging Profile as the Action in the Security &
   Visibility section.

## Identity-aware logins (SSO)

Integrate Island's identity management system with Coder's
authentication mechanisms to enable identity-aware logins.

### Configure single sign-on (SSO) seamless authentication between Coder and Island

Configure the same identity provider (IdP) for both your Island and Coder
deployment. Upon initial login to the Island browser, the user's session
token will automatically be passed to Coder and authenticate their Coder
session.
