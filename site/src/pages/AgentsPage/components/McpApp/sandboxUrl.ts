/**
 * Sandbox origin and resource helpers for MCP Apps. The sandbox proxy
 * document is served by coderd on a reserved wildcard subdomain
 * `mcpapp-<hex32>.<wildcard suffix>`, so every (server, chat) pair gets its
 * own origin.
 */

export const MCP_APP_MIME_TYPE = "text/html;profile=mcp-app";

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

/** Strips the leading `*.` from a wildcard hostname such as `*.apps.dev`. */
const wildcardSuffix = (wildcardHostname: string): string | undefined => {
	const trimmed = wildcardHostname.trim();
	if (!trimmed) {
		return undefined;
	}
	const suffix = trimmed.startsWith("*.") ? trimmed.slice(2) : trimmed;
	return suffix || undefined;
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
	const suffix = wildcardHostname
		? wildcardSuffix(wildcardHostname)
		: undefined;
	if (!suffix) {
		return undefined;
	}
	const query = csp ? `?csp=${encodeURIComponent(JSON.stringify(csp))}` : "";
	return `${protocol}//mcpapp-${label}.${suffix}/${query}`;
};

type McpAppViewResource =
	| {
			html: string;
			csp?: McpAppCsp;
			permissions?: McpAppPermissions;
			prefersBorder?: boolean;
	  }
	| { error: string };

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null && !Array.isArray(value);

const stringArray = (value: unknown): string[] | undefined =>
	Array.isArray(value) && value.every((item) => typeof item === "string")
		? value
		: undefined;

const parseCsp = (value: unknown): McpAppCsp | undefined => {
	if (!isRecord(value)) {
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

const parsePermissions = (value: unknown): McpAppPermissions | undefined => {
	if (!isRecord(value)) {
		return undefined;
	}
	const permissions: McpAppPermissions = {};
	if (isRecord(value.camera)) permissions.camera = value.camera;
	if (isRecord(value.microphone)) permissions.microphone = value.microphone;
	if (isRecord(value.geolocation)) permissions.geolocation = value.geolocation;
	if (isRecord(value.clipboardWrite)) {
		permissions.clipboardWrite = value.clipboardWrite;
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
	if (!isRecord(readResourceResult)) {
		return { error: "The MCP server returned an invalid resource." };
	}
	const contents = readResourceResult.contents;
	const first = Array.isArray(contents) ? contents[0] : undefined;
	if (!isRecord(first)) {
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
	const ui = isRecord(first._meta) ? first._meta.ui : undefined;
	const meta = isRecord(ui) ? ui : {};
	return {
		html,
		csp: parseCsp(meta.csp),
		permissions: parsePermissions(meta.permissions),
		prefersBorder:
			typeof meta.prefersBorder === "boolean" ? meta.prefersBorder : undefined,
	};
};
