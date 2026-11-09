import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceSubAgent,
} from "testHelpers/entities";
import {
	getAppHref,
	isBareOrigin,
	isSessionTokenDestinationTrusted,
	SESSION_TOKEN_PLACEHOLDER,
} from "./apps";
import sessionTokenDestinationCases from "./sessionTokenDestinationCases.json";

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
			`/@${MockWorkspace.owner_name}/Test-Workspace.a-workspace-agent/terminal?app=${app.slug}`,
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

describe("isSessionTokenDestinationTrusted", () => {
	// cli/open_internal_test.go runs the same cases against `coder open app`.
	it.each(sessionTokenDestinationCases.cases)(
		"$name (trusted: $trusted)",
		({ url, trusted, origin }) => {
			expect(
				isSessionTokenDestinationTrusted(
					url,
					origin ?? sessionTokenDestinationCases.origin,
				),
			).toBe(trusted);
		},
	);

	it("allows links to the current origin", () => {
		const url = `vscode://coder.coder-remote/open?url=${location.origin}&token=${SESSION_TOKEN_PLACEHOLDER}`;
		expect(isSessionTokenDestinationTrusted(url, undefined)).toBe(true);
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
