import { describe, expect, it } from "vitest";
import boardHtml from "#/testHelpers/mcpAppBoard.html?raw";
import {
	buildIframeAllow,
	buildSandboxUrl,
	MCP_APP_MIME_TYPE,
	resolveViewResource,
	sandboxHostLabel,
} from "./sandboxUrl";

describe("sandboxHostLabel", () => {
	it("returns the first 32 hex characters of a SHA-256 digest", async () => {
		const label = await sandboxHostLabel("server-1", "chat-1");
		expect(label).toMatch(/^[0-9a-f]{16}$/);
	});

	it("is deterministic and depends on both inputs", async () => {
		const [a, b, c, d] = await Promise.all([
			sandboxHostLabel("server-1", "chat-1"),
			sandboxHostLabel("server-1", "chat-1"),
			sandboxHostLabel("server-2", "chat-1"),
			sandboxHostLabel("server-1", "chat-2"),
		]);
		expect(a).toBe(b);
		expect(a).not.toBe(c);
		expect(a).not.toBe(d);
	});
});

describe("buildSandboxUrl", () => {
	const label = "0123456789abcdef0123456789abcdef";

	it("replaces the wildcard label with the reserved host label", () => {
		expect(
			buildSandboxUrl({
				wildcardHostname: "*.apps.example.com",
				label,
				protocol: "https:",
			}),
		).toBe(`https://mcp-${label}.apps.example.com/`);
		expect(
			buildSandboxUrl({
				wildcardHostname: "*-apps.example.com",
				label,
				protocol: "https:",
			}),
		).toBe(`https://mcp-${label}-apps.example.com/`);
	});

	it("encodes the declared csp as a query parameter", () => {
		const csp = { connectDomains: ["https://api.example.com"] };
		const url = buildSandboxUrl({
			wildcardHostname: "*.apps.example.com",
			label,
			csp,
			protocol: "https:",
		});
		expect(url).toBeDefined();
		const parsed = new URL(url ?? "");
		expect(parsed.origin).toBe(`https://mcp-${label}.apps.example.com`);
		expect(JSON.parse(parsed.searchParams.get("csp") ?? "")).toEqual(csp);
	});

	it("returns undefined without a wildcard hostname", () => {
		expect(buildSandboxUrl({ wildcardHostname: undefined, label })).toBe(
			undefined,
		);
		expect(buildSandboxUrl({ wildcardHostname: "", label })).toBe(undefined);
		expect(buildSandboxUrl({ wildcardHostname: "*", label })).toBe(undefined);
		expect(
			buildSandboxUrl({ wildcardHostname: "apps.example.com", label }),
		).toBe(undefined);
	});
});

describe("buildIframeAllow", () => {
	it("maps declared permissions to Permission Policy features", () => {
		expect(
			buildIframeAllow({ camera: {}, clipboardWrite: {}, geolocation: {} }),
		).toBe("camera; geolocation; clipboard-write");
		expect(buildIframeAllow({})).toBeUndefined();
		expect(buildIframeAllow(undefined)).toBeUndefined();
	});
});

describe("resolveViewResource", () => {
	const contents = (overrides: Record<string, unknown>) => ({
		contents: [
			{
				uri: "ui://taskboard/board",
				mimeType: MCP_APP_MIME_TYPE,
				...overrides,
			},
		],
	});

	it("returns the html text and ui metadata", () => {
		const resolved = resolveViewResource(
			contents({
				text: boardHtml,
				_meta: {
					ui: {
						csp: { connectDomains: ["https://api.example.com"] },
						permissions: { clipboardWrite: {} },
						prefersBorder: true,
					},
				},
			}),
		);
		expect(resolved).toEqual({
			html: boardHtml,
			csp: { connectDomains: ["https://api.example.com"] },
			permissions: { clipboardWrite: {} },
			prefersBorder: true,
		});
	});

	it("decodes base64 blobs as UTF-8", () => {
		const html = "<!doctype html><p>héllo</p>";
		const blob = btoa(String.fromCodePoint(...new TextEncoder().encode(html)));
		expect(resolveViewResource(contents({ blob }))).toEqual({
			html,
			csp: undefined,
			permissions: undefined,
			prefersBorder: undefined,
		});
	});

	it("rejects other mime types", () => {
		const resolved = resolveViewResource(
			contents({ mimeType: "text/html", text: boardHtml }),
		);
		expect(resolved).toEqual({
			error: expect.stringContaining('"text/html"'),
		});
	});

	it("rejects resources without html content", () => {
		expect(resolveViewResource(contents({}))).toEqual({
			error: "The MCP app resource has no HTML content.",
		});
		expect(resolveViewResource({ contents: [] })).toEqual({
			error: "The MCP server returned no resource contents.",
		});
		expect(resolveViewResource(null)).toEqual({
			error: "The MCP server returned an invalid resource.",
		});
	});
});
