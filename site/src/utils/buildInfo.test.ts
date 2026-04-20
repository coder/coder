import type { BuildInfoResponse } from "#/api/typesGenerated";
import { getPrereleaseFlag } from "./buildInfo";

const baseBuildInfo: BuildInfoResponse = {
	agent_api_version: "1.0",
	provisioner_api_version: "1.1",
	external_url: "https://github.com/coder/coder",
	version: "",
	dashboard_url: "https://example.com",
	workspace_proxy: false,
	upgrade_message: "",
	deployment_id: "test",
	telemetry: false,
};

describe("getPrereleaseFlag", () => {
	it("returns devel for -devel versions", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.16.0-devel+abc123",
			}),
		).toBe("devel");
	});

	it("returns devel for bare -devel versions", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.32.0-devel",
			}),
		).toBe("devel");
	});

	it("returns devel for v0.0.0", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v0.0.0",
			}),
		).toBe("devel");
	});

	it("returns undefined for release versions", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.16.0",
			}),
		).toBeUndefined();
	});

	it("returns rc for RC versions", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.32.0-rc.1",
			}),
		).toBe("rc");
	});

	it("returns devel when version contains both rc and -devel (devel wins)", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.33.0-rc.1-devel+727ec00f7",
			}),
		).toBe("devel");
	});

	it("returns undefined for empty version", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "",
			}),
		).toBeUndefined();
	});

	it("returns rc for -rc.0 and build metadata", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.32.0-rc.0",
			}),
		).toBe("rc");
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.32.0-rc.1+abc123",
			}),
		).toBe("rc");
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.32.0-rc.12",
			}),
		).toBe("rc");
	});

	it("returns undefined when rc segment lacks a dot", () => {
		expect(
			getPrereleaseFlag({
				...baseBuildInfo,
				version: "v2.32.0-rc",
			}),
		).toBeUndefined();
	});
});
