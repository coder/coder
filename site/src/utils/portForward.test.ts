import {
	portForwardURL,
	resolveLocalhostPort,
	rewriteLocalhostURL,
} from "./portForward";

describe("port forward URL", () => {
	const proxyHostWildcard = "*.proxy-host.tld";
	const samplePort = 12345;
	const sampleAgent = "my-agent";
	const sampleWorkspace = "my-workspace";
	const sampleUsername = "my-username";
	const sampleWorkspaceId = "0b9a2c7e-5f1d-4e3a-9c2b-7d4e6f8a1b3c";

	it("https, host and port", () => {
		const forwarded = portForwardURL(
			proxyHostWildcard,
			samplePort,
			sampleAgent,
			sampleWorkspace,
			sampleUsername,
			sampleWorkspaceId,
			"https",
		);
		expect(forwarded).toEqual(
			"http://12345s--my-agent--my-workspace--my-username.proxy-host.tld/",
		);
	});
	it("http, host, port and path", () => {
		const forwarded = portForwardURL(
			proxyHostWildcard,
			samplePort,
			sampleAgent,
			sampleWorkspace,
			sampleUsername,
			sampleWorkspaceId,
			"http",
			"/path1/path2",
		);
		expect(forwarded).toEqual(
			"http://12345--my-agent--my-workspace--my-username.proxy-host.tld/path1/path2",
		);
	});
	it("https, host, port, path and empty params", () => {
		const forwarded = portForwardURL(
			proxyHostWildcard,
			samplePort,
			sampleAgent,
			sampleWorkspace,
			sampleUsername,
			sampleWorkspaceId,
			"https",
			"/path1/path2",
			"?",
		);
		expect(forwarded).toEqual(
			"http://12345s--my-agent--my-workspace--my-username.proxy-host.tld/path1/path2?",
		);
	});
	it("http, host, port, path and query params", () => {
		const forwarded = portForwardURL(
			proxyHostWildcard,
			samplePort,
			sampleAgent,
			sampleWorkspace,
			sampleUsername,
			sampleWorkspaceId,
			"http",
			"/path1/path2",
			"?key1=value1&key2=value2",
		);
		expect(forwarded).toEqual(
			"http://12345--my-agent--my-workspace--my-username.proxy-host.tld/path1/path2?key1=value1&key2=value2",
		);
	});
	it("uses the short form when the long form does not fit", () => {
		const longName = "abcdefghijklmnopqrstuvwxyzabcdef";
		const forwarded = portForwardURL(
			proxyHostWildcard,
			8080,
			"main",
			`${longName}-${longName}`,
			sampleUsername,
			sampleWorkspaceId,
			"https",
		);
		// Same value as the Go test in coderd/workspaceapps/appurl.
		expect(forwarded).toEqual(
			"http://8080s--0oq73xnihgtc6v280bd0muhq4--1t30hzc.proxy-host.tld/",
		);
	});
	it("counts the fixed part of the wildcard label", () => {
		const forwarded = portForwardURL(
			"*--a-very-long-fixed-suffix.proxy-host.tld",
			8080,
			"main",
			sampleWorkspace,
			sampleUsername,
			sampleWorkspaceId,
			"http",
		);
		expect(forwarded).toEqual(
			"http://8080--0oq73xnihgtc6v280bd0muhq4--1t30hzc--a-very-long-fixed-suffix.proxy-host.tld/",
		);
	});
});

describe("resolveLocalhostPort", () => {
	it("returns parsed port when port string is non-empty", () => {
		expect(resolveLocalhostPort("3000", "http:")).toBe(3000);
	});

	it("defaults to 80 for http when port is empty", () => {
		expect(resolveLocalhostPort("", "http:")).toBe(80);
	});

	it("defaults to 443 for https when port is empty", () => {
		expect(resolveLocalhostPort("", "https:")).toBe(443);
	});
});

describe("rewriteLocalhostURL", () => {
	const proxyHost = "*.proxy-host.tld";
	const agent = "my-agent";
	const workspace = "my-workspace";
	const username = "my-username";
	const workspaceId = "0b9a2c7e-5f1d-4e3a-9c2b-7d4e6f8a1b3c";

	it("rewrites http://localhost:3000/path", () => {
		const result = rewriteLocalhostURL(
			"http://localhost:3000/path",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://3000--my-agent--my-workspace--my-username.proxy-host.tld/path",
		);
	});

	it("rewrites https://localhost:8443/path with s suffix", () => {
		const result = rewriteLocalhostURL(
			"https://localhost:8443/path",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://8443s--my-agent--my-workspace--my-username.proxy-host.tld/path",
		);
	});

	it("defaults to port 80 when no port is specified", () => {
		const result = rewriteLocalhostURL(
			"http://localhost/path",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://80--my-agent--my-workspace--my-username.proxy-host.tld/path",
		);
	});

	it("defaults to port 443 for https without explicit port", () => {
		const result = rewriteLocalhostURL(
			"https://localhost/path",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://443s--my-agent--my-workspace--my-username.proxy-host.tld/path",
		);
	});

	it("rewrites 127.0.0.1", () => {
		const result = rewriteLocalhostURL(
			"http://127.0.0.1:5000/api",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://5000--my-agent--my-workspace--my-username.proxy-host.tld/api",
		);
	});

	it("rewrites 0.0.0.0", () => {
		const result = rewriteLocalhostURL(
			"http://0.0.0.0:8080/",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://8080--my-agent--my-workspace--my-username.proxy-host.tld/",
		);
	});

	it("preserves query params", () => {
		const result = rewriteLocalhostURL(
			"http://localhost:3000/path?key=value",
			proxyHost,
			agent,
			workspace,
			username,
			workspaceId,
		);
		expect(result).toEqual(
			"http://3000--my-agent--my-workspace--my-username.proxy-host.tld/path?key=value",
		);
	});

	it("returns non-localhost URLs unchanged", () => {
		const url = "https://example.com/page";
		expect(
			rewriteLocalhostURL(
				url,
				proxyHost,
				agent,
				workspace,
				username,
				workspaceId,
			),
		).toBe(url);
	});

	it("returns invalid URLs unchanged", () => {
		const url = "not-a-url";
		expect(
			rewriteLocalhostURL(
				url,
				proxyHost,
				agent,
				workspace,
				username,
				workspaceId,
			),
		).toBe(url);
	});
});
