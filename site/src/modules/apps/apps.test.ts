import type { WorkspaceApp } from "#/api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceSubAgent,
} from "#/testHelpers/entities";
import {
	getAppHref,
	getVSCodeHref,
	isAppBlockedByMissingWildcard,
	isAppUrlValid,
	isBareOrigin,
	isSessionTokenDestinationTrusted,
	isWorkspaceAppEmbeddable,
	openAppInNewWindow,
	SESSION_TOKEN_PLACEHOLDER,
} from "./apps";

function buildApp(overrides: Partial<WorkspaceApp> = {}): WorkspaceApp {
	return {
		...MockWorkspaceApp,
		health: "healthy",
		...overrides,
	};
}

describe("getVSCodeHref", () => {
	it("includes the chat ID when provided", () => {
		const folder = "/workspace/test";
		const href = getVSCodeHref("vscode", {
			owner: MockWorkspace.owner_name,
			workspace: MockWorkspace.name,
			token: "user-session-token",
			agent: MockWorkspaceAgent.name,
			folder,
			chatId: "chat-123",
		});
		const query = new URLSearchParams({
			owner: MockWorkspace.owner_name,
			workspace: MockWorkspace.name,
			url: location.origin,
			token: "user-session-token",
			openRecent: "true",
			agent: MockWorkspaceAgent.name,
			folder,
			chatId: "chat-123",
		});

		expect(href).toBe(`vscode://coder.coder-remote/open?${query}`);
	});

	it("omits the chat ID when none is provided", () => {
		const href = getVSCodeHref("cursor", {
			owner: MockWorkspace.owner_name,
			workspace: MockWorkspace.name,
			token: "user-session-token",
		});
		const query = new URLSearchParams({
			owner: MockWorkspace.owner_name,
			workspace: MockWorkspace.name,
			url: location.origin,
			token: "user-session-token",
			openRecent: "true",
		});

		expect(href).toBe(`cursor://coder.coder-remote/open?${query}`);
	});
});

describe("getAppHref", () => {
	it("returns the URL without changes when external app has regular URL", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: "https://example.com",
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
		});
		expect(href).toBe(externalApp.url);
	});

	it("returns the URL with the session token replaced when external app needs session token", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: `vscode://coder.coder-remote/open?url=${location.origin}&token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			token: "user-session-token",
		});
		expect(href).toBe(
			`vscode://coder.coder-remote/open?url=${location.origin}&token=user-session-token`,
		);
	});

	it("doesn't replace the session token when the link points to another deployment", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: `vscode://coder.coder-remote/open?url=https://attacker.example&token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			token: "user-session-token",
		});
		expect(href).toBe(externalApp.url);
	});

	it("strips the session token for sub-agent apps", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: `vscode://coder.coder-remote/open?url=${location.origin}&token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceSubAgent,
			workspace: MockWorkspace,
			token: "user-session-token",
		});
		expect(href).toBe(
			`vscode://coder.coder-remote/open?url=${location.origin}&token=`,
		);
	});

	it("replaces the session token for Antigravity IDE URLs", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: `antigravity-ide://coder.coder-remote/open?url=${location.origin}&token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			token: "user-session-token",
		});
		expect(href).toBe(
			`antigravity-ide://coder.coder-remote/open?url=${location.origin}&token=user-session-token`,
		);
	});

	it("doesn't return the URL with the session token replaced when using the HTTP protocol", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: `https://example.com?token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			token: "user-session-token",
		});
		expect(href).toBe(externalApp.url);
	});

	it("doesn't return the URL with the session token replaced when using unauthorized protocol", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: `ftp://example.com?token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			path: "/path-base",
			token: "user-session-token",
		});
		expect(href).toBe(externalApp.url);
	});

	it("returns a path when app doesn't use a subdomain", () => {
		const app = {
			...MockWorkspaceApp,
			subdomain: false,
		};
		const href = getAppHref(app, {
			host: "*.apps-host.tld",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			path: "/path-base",
		});
		expect(href).toBe(
			`/path-base/@${MockWorkspace.owner_name}/test-workspace.a-workspace-agent/apps/${app.slug}/`,
		);
	});

	it("includes the app slug in the URL when app has a command", () => {
		const app = {
			...MockWorkspaceApp,
			command: "ls -la",
		};
		const href = getAppHref(app, {
			host: "*.apps-host.tld",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			path: "",
		});
		expect(href).toBe(
			`/@${MockWorkspace.owner_name}/test-workspace.a-workspace-agent/terminal?app=${app.slug}`,
		);
	});

	it("uses the subdomain when app has a subdomain", () => {
		const app = {
			...MockWorkspaceApp,
			subdomain: true,
			subdomain_name: "hellocoder",
		};
		const href = getAppHref(app, {
			host: "*.apps-host.tld",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			path: "/path-base",
		});
		expect(href).toBe("http://hellocoder.apps-host.tld/");
	});

	it("returns a path when app has a subdomain but no subdomain name", () => {
		const app = {
			...MockWorkspaceApp,
			subdomain: true,
			subdomain_name: undefined,
		};
		const href = getAppHref(app, {
			host: "*.apps-host.tld",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			path: "/path-base",
		});
		expect(href).toBe(
			`/path-base/@${MockWorkspace.owner_name}/test-workspace.a-workspace-agent/apps/${app.slug}/`,
		);
	});

	it("returns the raw URL without throwing when external app has an invalid URL", () => {
		const externalApp = {
			...MockWorkspaceApp,
			external: true,
			url: "my-repo",
		};
		let href = "";
		expect(() => {
			href = getAppHref(externalApp, {
				host: "*.apps-host.tld",
				path: "/path-base",
				agent: MockWorkspaceAgent,
				workspace: MockWorkspace,
				token: "user-session-token",
			});
		}).not.toThrow();
		expect(href).toBe("my-repo");
	});
});

describe("isSessionTokenDestinationTrusted", () => {
	const origin = location.origin;
	const token = SESSION_TOKEN_PLACEHOLDER;

	it.each([
		[
			"VS Code",
			`vscode://coder.coder-remote/open?owner=a&workspace=b&url=${origin}&token=${token}`,
		],
		[
			"trailing slash",
			`cursor://coder.coder-remote/open?url=${origin}/&token=${token}`,
		],
		[
			"JetBrains Toolbox",
			`jetbrains://gateway/coder?&workspace=b&url=${origin}&token=${token}&ide_product_code=GO`,
		],
		[
			"JetBrains Gateway",
			`jetbrains-gateway://connect#type=coder&workspace=b&url=${origin}&token=${token}`,
		],
		[
			"dev container",
			`cursor://coder.coder-remote/openDevContainer?url=${origin}&token=${token}`,
		],
		[
			"JetBrains Toolbox (documented path)",
			`jetbrains://gateway/com.coder.toolbox?url=${origin}&token=${token}`,
		],
		[
			"uppercase scheme and authority",
			`VSCode://Coder.Coder-Remote/open?url=${origin}&token=${token}`,
		],
	])("allows %s links to this deployment", (_, url) => {
		expect(isSessionTokenDestinationTrusted(url, undefined)).toBe(true);
	});

	it.each([
		[
			"another origin",
			`vscode://coder.coder-remote/open?url=https://attacker.example&token=${token}`,
		],
		[
			"another port",
			`vscode://coder.coder-remote/open?url=${location.protocol}//${location.hostname}:1&token=${token}`,
		],
		[
			"an encoded other origin",
			`vscode://coder.coder-remote/open?url=https%3A%2F%2Fattacker.example&token=${token}`,
		],
		["no url", `vscode://coder.coder-remote/open?token=${token}`],
		[
			"an unparsable url",
			`vscode://coder.coder-remote/open?url=not-a-url&token=${token}`,
		],
		[
			"duplicate url",
			`vscode://coder.coder-remote/open?url=${origin}&url=https://attacker.example&token=${token}`,
		],
		[
			"url in query and fragment",
			`jetbrains-gateway://connect?url=https://attacker.example#url=${origin}&token=${token}`,
		],
		[
			"the token outside the token param",
			`jetbrains://gateway/coder?url=${origin}&token=x&ide_download_link=https://attacker.example/${token}`,
		],
		[
			"the token twice",
			`vscode://coder.coder-remote/open?url=${origin}&token=${token}&t=${token}`,
		],
		[
			"the token in a VS Code fragment",
			`vscode://coder.coder-remote/open?url=${origin}#token=${token}`,
		],
		[
			"a url in the fragment",
			`vscode://coder.coder-remote/open?url=${origin}&token=${token}#url=https://attacker.example`,
		],
		[
			"another extension",
			`vscode://attacker.ext/open?url=${origin}&token=${token}`,
		],
		["an unknown scheme", `coder://open?url=${origin}&token=${token}`],
		[
			"a workspace app path",
			`vscode://coder.coder-remote/open?url=${origin}/@victim/ws.main/apps/evil/&token=${token}`,
		],
		[
			"a double-encoded host",
			`vscode://coder.coder-remote/open?url=https://attacker.example%252Fx@${location.host}/&token=${token}`,
		],
		[
			"a double-encoded url param",
			`vscode://coder.coder-remote/open?owner=x%26url%3Dhttps%253A%252F%252Fattacker.example&url=${origin}&token=${token}`,
		],
		[
			"a triple-encoded url param",
			`vscode://coder.coder-remote/open?x=%252526url%25253Dhttps%25253A%25252F%25252Fattacker.example&url=${origin}&token=${token}`,
		],
		[
			"an opaque handler",
			`vscode:coder.coder-remote/open?url=${origin}&token=${token}`,
		],
		[
			"userinfo in the handler",
			`vscode://x@coder.coder-remote/open?url=${origin}&token=${token}`,
		],
		[
			"dot segments in the handler",
			`vscode://coder.coder-remote/open/../open?url=${origin}&token=${token}`,
		],
		[
			"params in a VS Code fragment",
			`vscode://coder.coder-remote/open#url=${origin}&token=${token}`,
		],
		[
			"params in a JetBrains Gateway query",
			`jetbrains-gateway://connect?url=${origin}&token=${token}`,
		],
		[
			"the placeholder outside the raw token entry",
			`jetbrains-gateway://connect#type=coder&url=${origin}&token=%24SESSION_TOKEN&ide_download_link=https://attacker.example/${token}`,
		],
		[
			"an encoded url name",
			`jetbrains-gateway://connect#type=coder&%75rl=${origin}&token=${token}`,
		],
		[
			"a non-Coder JetBrains Gateway provider",
			`jetbrains-gateway://connect#type=ssh&host=attacker.example&url=${origin}&token=${token}`,
		],
		[
			"a duplicate JetBrains Gateway type",
			`jetbrains-gateway://connect#type=coder&url=${origin}&token=${token}&type=ssh`,
		],
		[
			"params after a fragment",
			`vscode://coder.coder-remote/open?x=1#&url=${origin}&token=${token}`,
		],
		[
			"a non-ASCII scheme",
			`\u212Airo://coder.coder-remote/open?url=${origin}&token=${token}`,
		],
		...["\t", "\n", "\r"].map((char) => [
			`${JSON.stringify(char)} in a param name`,
			`jetbrains-gateway://connect#type=coder&url=${origin}&ur${char}l=https://attacker.example&token=${token}`,
		]),
	])("rejects links with %s", (_, url) => {
		expect(isSessionTokenDestinationTrusted(url, undefined)).toBe(false);
	});

	it("allows links to the access URL when the dashboard uses another host", () => {
		const url = `vscode://coder.coder-remote/open?url=https://coder.example.com&token=${token}`;
		expect(
			isSessionTokenDestinationTrusted(url, "https://coder.example.com/"),
		).toBe(true);
		expect(
			isSessionTokenDestinationTrusted(url, "https://other.example.com"),
		).toBe(false);
	});
});

describe("isBareOrigin", () => {
	it.each(["https://coder.example.com", "http://localhost:3000/"])(
		"allows %s",
		(url) => {
			expect(isBareOrigin(url)).toBe(true);
		},
	);

	it.each([
		"https://coder.example.com/@o/w.a/apps/x/",
		"https://attacker.example@coder.example.com",
		"https://coder.example.com/?x=1",
		"https://coder.example.com/#x",
		"https://coder.example.com/.",
		"https://coder.example.com ",
		"not-a-url",
	])("rejects %s", (url) => {
		expect(isBareOrigin(url)).toBe(false);
	});
});

describe("isAppUrlValid", () => {
	it("returns false for an external app with an unparsable URL", () => {
		expect(isAppUrlValid(buildApp({ external: true, url: "my-repo" }))).toBe(
			false,
		);
	});

	it("returns true for an external app with a valid HTTP URL", () => {
		expect(
			isAppUrlValid(buildApp({ external: true, url: "https://example.com" })),
		).toBe(true);
	});

	it("returns true for an external app with a valid custom scheme", () => {
		expect(
			isAppUrlValid(buildApp({ external: true, url: "vscode://open" })),
		).toBe(true);
	});

	it("returns true for non-external apps", () => {
		expect(isAppUrlValid(buildApp({ external: false }))).toBe(true);
	});
});

describe("openAppInNewWindow", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("severs opener and navigates popup to href on success", () => {
		const popup = {
			opener: window,
			location: { href: "" },
		};
		vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);

		openAppInNewWindow("https://app.example.com");

		expect(popup.opener).toBeNull();
		expect(popup.location.href).toBe("https://app.example.com");
	});

	it("still navigates when nulling opener throws", () => {
		const popup = {
			location: { href: "" },
		};
		Object.defineProperty(popup, "opener", {
			set() {
				throw new Error("Electron restriction");
			},
			get() {
				return window;
			},
		});
		vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);

		openAppInNewWindow("https://app.example.com");

		expect(popup.location.href).toBe("https://app.example.com");
	});
});

describe("isWorkspaceAppEmbeddable", () => {
	it("returns true for visible path-based apps", () => {
		expect(isWorkspaceAppEmbeddable(buildApp())).toBe(true);
	});

	it("returns false for command apps, hidden apps, and external apps", () => {
		expect(isWorkspaceAppEmbeddable(buildApp({ command: "run-preview" }))).toBe(
			false,
		);
		expect(isWorkspaceAppEmbeddable(buildApp({ hidden: true }))).toBe(false);
		expect(
			isWorkspaceAppEmbeddable(
				buildApp({ external: true, url: "https://example.com" }),
			),
		).toBe(false);
	});
});

describe("isAppBlockedByMissingWildcard", () => {
	it("blocks subdomain apps when no wildcard host is configured", () => {
		const subdomainApp = buildApp({ subdomain: true });

		expect(isAppBlockedByMissingWildcard(subdomainApp, "")).toBe(true);
		expect(isAppBlockedByMissingWildcard(subdomainApp, undefined)).toBe(true);
		expect(
			isAppBlockedByMissingWildcard(subdomainApp, "*.apps.example.com"),
		).toBe(false);
	});

	it("never blocks path-based apps", () => {
		const pathApp = buildApp({ subdomain: false });

		expect(isAppBlockedByMissingWildcard(pathApp, "")).toBe(false);
		expect(isAppBlockedByMissingWildcard(pathApp, undefined)).toBe(false);
	});
});
