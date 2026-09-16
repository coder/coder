/**
 * Sandbox origin and resource helpers for MCP Apps. The sandbox proxy
 * document is served by coderd on a reserved wildcard subdomain: the `*` of
 * the wildcard hostname is replaced by `mcpapp-<hex32>`, so every
 * (server, chat) pair gets its own origin.
 */

import { asRecord } from "../ChatElements/runtimeTypeUtils";

export const MCP_APP_MIME_TYPE = "text/html;profile=mcp-app";

/**
 * Sandbox flags for the proxy iframe rendered by the dashboard. The proxy
 * document runs its own script and must keep its origin for the CSP header
 * and the origin check on the message channel.
 */
export const MCP_APP_PROXY_FRAME_SANDBOX = "allow-scripts allow-same-origin";

/**
 * Sandbox flags the proxy applies to the inner view iframe, sent in
 * `ui/notifications/sandbox-resource-ready`. Forms are deliberately not
 * allowed.
 */
export const MCP_APP_VIEW_SANDBOX = "allow-scripts allow-same-origin";

export type McpAppCsp = {
	connectDomains?: string[];
	resourceDomains?: string[];
	frameDomains?: string[];
	baseUriDomains?: string[];
};

export type McpAppPermissions = {
	camera?: object;
	microphone?: object;
	geolocation?: object;
	clipboardWrite?: object;
};

const PERMISSION_POLICY_FEATURES: ReadonlyArray<
	[key: keyof McpAppPermissions, feature: string]
> = [
	["camera", "camera"],
	["microphone", "microphone"],
	["geolocation", "geolocation"],
	["clipboardWrite", "clipboard-write"],
];

/** Builds the iframe `allow` attribute for the permissions a view declared. */
export const buildIframeAllow = (
	permissions: McpAppPermissions | undefined,
): string | undefined => {
	if (!permissions) {
		return undefined;
	}
	const features = PERMISSION_POLICY_FEATURES.filter(
		([key]) => permissions[key] !== undefined,
	).map(([, feature]) => feature);
	return features.length > 0 ? features.join("; ") : undefined;
};

const hex = (bytes: ArrayBuffer): string =>
	Array.from(new Uint8Array(bytes), (byte) =>
		byte.toString(16).padStart(2, "0"),
	).join("");

/** First 32 hex characters of SHA-256 over `<serverId>:<chatId>`. */
export const sandboxHostLabel = async (
	mcpServerConfigId: string,
	chatId: string,
): Promise<string> => {
	const digest = await crypto.subtle.digest(
		"SHA-256",
		new TextEncoder().encode(`${mcpServerConfigId}:${chatId}`),
	);
	return hex(digest).slice(0, 32);
};

/** Replaces the wildcard label with the reserved `mcpapp-<label>` host label. */
const sandboxHost = (
	wildcardHostname: string,
	label: string,
): string | undefined => {
	const trimmed = wildcardHostname.trim();
	if (!trimmed.startsWith("*") || trimmed.length === 1) {
		return undefined;
	}
	return `mcpapp-${label}${trimmed.slice(1)}`;
};

export const buildSandboxUrl = ({
	wildcardHostname,
	label,
	csp,
	protocol = window.location.protocol,
}: {
	wildcardHostname: string | undefined;
	label: string;
	csp?: McpAppCsp;
	protocol?: string;
}): string | undefined => {
	const host = wildcardHostname
		? sandboxHost(wildcardHostname, label)
		: undefined;
	if (!host) {
		return undefined;
	}
	const query = csp ? `?csp=${encodeURIComponent(JSON.stringify(csp))}` : "";
	return `${protocol}//${host}/${query}`;
};

type McpAppViewResource =
	| {
			html: string;
			csp?: McpAppCsp;
			permissions?: McpAppPermissions;
			prefersBorder?: boolean;
	  }
	| { error: string };

const stringArray = (value: unknown): string[] | undefined =>
	Array.isArray(value) && value.every((item) => typeof item === "string")
		? value
		: undefined;

const parseCsp = (rawValue: unknown): McpAppCsp | undefined => {
	const value = asRecord(rawValue);
	if (!value) {
		return undefined;
	}
	const csp: McpAppCsp = {};
	const connectDomains = stringArray(value.connectDomains);
	const resourceDomains = stringArray(value.resourceDomains);
	const frameDomains = stringArray(value.frameDomains);
	const baseUriDomains = stringArray(value.baseUriDomains);
	if (connectDomains) csp.connectDomains = connectDomains;
	if (resourceDomains) csp.resourceDomains = resourceDomains;
	if (frameDomains) csp.frameDomains = frameDomains;
	if (baseUriDomains) csp.baseUriDomains = baseUriDomains;
	return Object.keys(csp).length > 0 ? csp : undefined;
};

const parsePermissions = (rawValue: unknown): McpAppPermissions | undefined => {
	const value = asRecord(rawValue);
	if (!value) {
		return undefined;
	}
	const permissions: McpAppPermissions = {};
	for (const [key] of PERMISSION_POLICY_FEATURES) {
		const declared = asRecord(value[key]);
		if (declared) {
			permissions[key] = declared;
		}
	}
	return Object.keys(permissions).length > 0 ? permissions : undefined;
};

const decodeBase64Utf8 = (blob: string): string | undefined => {
	try {
		const binary = atob(blob);
		const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
		return new TextDecoder().decode(bytes);
	} catch {
		return undefined;
	}
};

/**
 * Extracts the view HTML and its `_meta.ui` settings from a raw MCP
 * `resources/read` result. Only the first content entry is considered.
 */
export const resolveViewResource = (
	readResourceResult: unknown,
): McpAppViewResource => {
	const result = asRecord(readResourceResult);
	if (!result) {
		return { error: "The MCP server returned an invalid resource." };
	}
	const first = Array.isArray(result.contents)
		? asRecord(result.contents[0])
		: null;
	if (!first) {
		return { error: "The MCP server returned no resource contents." };
	}
	if (first.mimeType !== MCP_APP_MIME_TYPE) {
		return {
			error: `Unsupported resource type ${
				typeof first.mimeType === "string" ? `"${first.mimeType}"` : "(missing)"
			}. MCP apps must use "${MCP_APP_MIME_TYPE}".`,
		};
	}
	let html: string | undefined;
	if (typeof first.text === "string") {
		html = first.text;
	} else if (typeof first.blob === "string") {
		html = decodeBase64Utf8(first.blob);
	}
	if (html === undefined) {
		return { error: "The MCP app resource has no HTML content." };
	}
	const meta = asRecord(asRecord(first._meta)?.ui) ?? {};
	return {
		html,
		csp: parseCsp(meta.csp),
		permissions: parsePermissions(meta.permissions),
		prefersBorder:
			typeof meta.prefersBorder === "boolean" ? meta.prefersBorder : undefined,
	};
};
