import {
	buildPresetURL,
	defaultMCPPresetOptions,
	detectBuiltInMCPServer,
	parsePresetURL,
} from "./mcpPresets";

describe("buildPresetURL", () => {
	it("leaves the query off for default Slack options", () => {
		expect(
			buildPresetURL(
				"slack",
				"https://coder.example.com/",
				defaultMCPPresetOptions,
			),
		).toBe("https://coder.example.com/api/experimental/mcp/builtin/slack");
	});

	it("adds the Slack options that differ from the defaults", () => {
		expect(
			buildPresetURL("slack", "https://coder.example.com", {
				...defaultMCPPresetOptions,
				slackAPIBaseURL: "https://slack-proxy.example.com/api",
				slackPinThread: true,
			}),
		).toBe(
			"https://coder.example.com/api/experimental/mcp/builtin/slack?api_base_url=https%3A%2F%2Fslack-proxy.example.com%2Fapi&pin_thread=true",
		);
	});

	it("lists the default GitHub toolsets", () => {
		expect(
			buildPresetURL(
				"github",
				"https://coder.example.com",
				defaultMCPPresetOptions,
			),
		).toBe(
			"https://coder.example.com/api/experimental/mcp/builtin/github?toolsets=context,repos,issues,pull_requests",
		);
	});

	it("collapses to all when every toolset is chosen", () => {
		expect(
			buildPresetURL("github", "https://coder.example.com", {
				...defaultMCPPresetOptions,
				githubToolsets: ["repos", "all"],
				githubReadOnly: true,
			}),
		).toBe(
			"https://coder.example.com/api/experimental/mcp/builtin/github?toolsets=all&read_only=true",
		);
	});
});

describe("parsePresetURL", () => {
	it("round-trips the options of a GitHub Enterprise server", () => {
		const options = {
			...defaultMCPPresetOptions,
			githubAPIBaseURL: "https://ghe.example.com/api/v3",
			githubToolsets: ["repos", "actions"],
			githubReadOnly: true,
		};
		const url = buildPresetURL("github", "https://coder.example.com", options);

		expect(parsePresetURL(url)).toEqual({
			preset: "github",
			origin: "https://coder.example.com",
			options,
		});
	});

	it("keeps a path prefix of the deployment URL", () => {
		expect(
			parsePresetURL(
				"https://example.com/coder/api/experimental/mcp/builtin/slack",
			)?.origin,
		).toBe("https://example.com/coder");
	});

	it.each(["https://api.githubcopilot.com/mcp/", "not a url"])(
		"does not recognize %s",
		(url) => {
			expect(parsePresetURL(url)).toBeUndefined();
		},
	);
});

describe("detectBuiltInMCPServer", () => {
	it("prefers the URL over the slug", () => {
		expect(
			detectBuiltInMCPServer({
				slug: "github-livecheck",
				url: "https://coder.example.com/api/experimental/mcp/builtin/github",
			}),
		).toBe("github");
	});

	it("falls back to the slug", () => {
		expect(detectBuiltInMCPServer({ slug: "slack" })).toBe("slack");
	});

	it("does not take other servers for built-in ones", () => {
		expect(
			detectBuiltInMCPServer({
				slug: "linear",
				url: "https://mcp.linear.app/mcp",
			}),
		).toBeUndefined();
	});
});
