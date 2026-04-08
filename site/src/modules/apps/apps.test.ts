import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import {
	getAppHref,
	getVSCodeHref,
	openAppInNewWindow,
	SESSION_TOKEN_PLACEHOLDER,
} from "./apps";

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
			url: `vscode://example.com?token=${SESSION_TOKEN_PLACEHOLDER}`,
		};
		const href = getAppHref(externalApp, {
			host: "*.apps-host.tld",
			path: "/path-base",
			agent: MockWorkspaceAgent,
			workspace: MockWorkspace,
			token: "user-session-token",
		});
		expect(href).toBe("vscode://example.com?token=user-session-token");
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
			`/path-base/@${MockWorkspace.owner_name}/Test-Workspace.a-workspace-agent/apps/${app.slug}/`,
		);
	});

	it("includes the command in the URL when app has a command", () => {
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
			`/@${MockWorkspace.owner_name}/Test-Workspace.a-workspace-agent/terminal?command=ls%20-la`,
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
			`/path-base/@${MockWorkspace.owner_name}/Test-Workspace.a-workspace-agent/apps/${app.slug}/`,
		);
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
