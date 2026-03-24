import {
	localHosts,
	portForwardURL,
	resolveLocalhostPort,
} from "./portForward";

describe("port forward URL", () => {
	const proxyHostWildcard = "*.proxy-host.tld";
	const samplePort = 12345;
	const sampleAgent = "my-agent";
	const sampleWorkspace = "my-workspace";
	const sampleUsername = "my-username";

	it("https, host and port", () => {
		const forwarded = portForwardURL(
			proxyHostWildcard,
			samplePort,
			sampleAgent,
			sampleWorkspace,
			sampleUsername,
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
			"http",
			"/path1/path2",
			"?key1=value1&key2=value2",
		);
		expect(forwarded).toEqual(
			"http://12345--my-agent--my-workspace--my-username.proxy-host.tld/path1/path2?key1=value1&key2=value2",
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

	it("works with protocol without colon", () => {
		expect(resolveLocalhostPort("", "http")).toBe(80);
	});

	it("works with protocol without colon for https", () => {
		expect(resolveLocalhostPort("", "https")).toBe(443);
	});
});

describe("localHosts", () => {
	it("contains localhost", () => {
		expect(localHosts.has("localhost")).toBe(true);
	});

	it("contains 127.0.0.1", () => {
		expect(localHosts.has("127.0.0.1")).toBe(true);
	});

	it("contains 0.0.0.0", () => {
		expect(localHosts.has("0.0.0.0")).toBe(true);
	});

	it("does not contain example.com", () => {
		expect(localHosts.has("example.com")).toBe(false);
	});
});
