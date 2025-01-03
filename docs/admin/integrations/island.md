# Island Browser Integration

<div>
  <a href="https://github.com/ericpaulsen" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Eric Paulsen</span>
    <img src="https://github.com/ericpaulsen.png" alt="ericpaulsen" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
April 24, 2024

---

[Island][] is an enterprise-grade browser, offering a Chromium-based experience
similar to popular web browsers like Chrome and Edge. It includes built-in
security features for corporate applications and data, aiming to bridge the gap
between consumer-focused browsers and the security needs of the enterprise.

Coder natively integrates with Island&rsquo;s feature set, which include data
loss protection (DLP), application awareness, browser session recording, and
single sign-on (SSO). This guide intends to document these feature categories
and how they apply to your Coder deployment.

## General Configuration

### Create an Application Group for Coder

We recommend creating an Application Group specific to Coder in the Island
Management console. This Application Group object will be referenced when
creating browser policies.

[See the Island documentation for creating an Application Group][app-group].

## Advanced Data Loss Protection

Integrate Island&rsquo;s advanced data loss prevention (DLP) capabilities with
Coder&rsquo;s cloud development environment (CDE), enabling you to control the
&ldquo;last mile&rdquo; between developers&rsquo; CDE and their local devices,
ensuring that sensitive IP remains in your centralized environment.

### Block cut, copy, paste, printing, screen share

1. [Create a Data Sandbox Profile][data-sandbox].

1. Configure the following actions to allow/block (based on your security
   requirements).

   - Screenshot and Screen Share
   - Printing
   - Save Page
   - Clipboard Limitations

1. [Create a Policy Rule][policy-rule] to apply the Data Sandbox Profile.

1. Define the Coder Application group as the Destination Object.

1. Define the Data Sandbox Profile as the Action in the Last Mile Protection
   section.

### Conditionally allow copy on Coder&rsquo;s CLI authentication page

1. [Create a URL Object][policy-rule] with the following configuration.

   - **Include**
   - **URL type**: Wildcard
   - **URL address**: `coder.example.com/cli-auth`
   - **Casing**: Insensitive

1. [Create a Data Sandbox Profile][data-sandbox].

1. Configure action to allow copy/paste.

1. [Create a Policy Rule][policy-rule] to apply the Data Sandbox Profile.

1. Define the URL Object you created as the Destination Object.

1. Define the Data Sandbox Profile as the Action in the Last Mile Protection
   section.

### Prevent file upload/download from the browser

1. Create a Protection Profiles for both upload/download.

   - [Upload documentation][upload-docs]
   - [Download documentation][download-docs]

1. [Create a Policy Rule][policy-rule] to apply the Protection Profiles.

1. Define the Coder Application group as the Destination Object.

1. Define the applicable Protection Profile as the Action in the Data Protection
   section.

### Scan files for sensitive data

1. [Create a Data Loss Prevention scanner][dlp-scanner].

1. [Create a Policy Rule][policy-rule] to apply the DLP Scanner.

1. Define the Coder Application group as the Destination Object.

1. Define the DLP Scanner as the Action in the Data Protection section.

## Application Awareness and Boundaries

Ensure that Coder is only accessed through the Island browser, guaranteeing that
your browser-level DLP policies are always enforced, and developers can&rsquo;t
sidestep such policies simply by using another browser.

### Configure browser enforcement, conditional access policies

1. Create a conditional access policy for your configured identity provider.

   <blockquote class="admonition">
   The configured IdP must be the same for both Coder and Island
   </blockquote>

   - [Azure Active Directory/Entra ID][island-entra]
   - [Okta][island-okta]
   - [Google][island-google]

## Browser Activity Logging

Govern and audit in-browser terminal and IDE sessions using Island, such as
screenshots, mouse clicks, and keystrokes.

### Activity Logging Module

1. [Create an Activity Logging Profile][logging-profile]. Supported browser
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

1. [Create a Policy Rule][policy-rule] to apply the Activity Logging Profile.

1. Define the Coder Application group as the Destination Object.

1. Define the Activity Logging Profile as the Action in the Security &
   Visibility section.

## Identity-aware logins (SSO)

Integrate Island&rsquo;s identity management system with Coder&rsquo;s
authentication mechanisms to enable identity-aware logins.

### Configure single sign-on (SSO) seamless authentication between Coder and Island

Configure the same identity provider (IdP) for both your Island and Coder
deployment. Upon initial login to the Island browser, the user&rsquo;s session
token will automatically be passed to Coder and authenticate their Coder
session.

<!-- Reference links -->

[island]: https://www.island.io/
[app-group]:
	https://documentation.island.io/docs/create-and-configure-an-application-group-object
[data-sandbox]:
	https://documentation.island.io/docs/create-and-configure-a-data-sandbox-profile
[policy-rule]:
	https://documentation.island.io/docs/create-and-configure-a-policy-rule-general
[url-object]:
	https://documentation.island.io/docs/create-and-configure-a-policy-rule-general
[logging-profile]:
	https://documentation.island.io/docs/create-and-configure-an-activity-logging-profile
[dlp-scanner]:
	https://documentation.island.io/docs/create-a-data-loss-prevention-scanner
[upload-docs]:
	https://documentation.island.io/docs/create-and-configure-an-upload-protection-profile
[download-docs]:
	https://documentation.island.io/v1/docs/en/create-and-configure-a-download-protection-profile
[island-entra]:
	https://documentation.island.io/docs/configure-browser-enforcement-for-island-with-azure-ad#create-and-apply-a-conditional-access-policy
[island-okta]:
	https://documentation.island.io/docs/configure-browser-enforcement-for-island-with-okta
[island-google]:
	https://documentation.island.io/docs/configure-browser-enforcement-for-island-with-google-enterprise
