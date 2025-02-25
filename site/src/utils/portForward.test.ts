import { portForwardURL } from "./portForward";

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
