// Claude Platform for AWS is Anthropic's Messages API hosted on AWS. It is an
// authentication method on the `anthropic` provider type rather than a
// provider type of its own, so these helpers are shared by the form, the
// nested fields component, and the API mapping.

export const CLAUDE_PLATFORM_DEFAULT_REGION = "us-east-1";

// Claude Platform for AWS is an authentication method on `anthropic`, not a
// provider type, so list rows and icons key off this synthetic value instead
// of a real AIProviderType.
export const CLAUDE_PLATFORM_DISPLAY_TYPE = "claude-platform-aws";

// AWS region names are lowercase alphanumerics and hyphens. The value is
// interpolated into the regional host, so a malformed region would otherwise
// produce an endpoint that no longer tracks later region edits.
export const CLAUDE_PLATFORM_REGION_REGEX = /^[a-z0-9-]+$/;

// Regional endpoint, e.g. https://aws-external-anthropic.{region}.api.aws
const CLAUDE_PLATFORM_URL_REGEX =
	/^https:\/\/aws-external-anthropic\.([a-z0-9-]+)\.api\.aws\/?$/i;

export const claudePlatformBaseUrl = (region: string) =>
	`https://aws-external-anthropic.${region}.api.aws`;

// Whether the endpoint is a generated regional host rather than an operator
// supplied proxy. Only generated values are rewritten when the region changes,
// so a proxy URL is never silently replaced.
export const isCanonicalClaudePlatformUrl = (baseUrl: string): boolean =>
	CLAUDE_PLATFORM_URL_REGEX.test(baseUrl.trim());
