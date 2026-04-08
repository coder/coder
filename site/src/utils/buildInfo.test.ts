import type { BuildInfoResponse } from "#/api/typesGenerated";
import { isDevBuild, isRcBuild } from "./buildInfo";

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

describe("isDevBuild", () => {
	it("returns true for -devel versions", () => {
		expect(
			isDevBuild({ ...baseBuildInfo, version: "v2.16.0-devel+abc123" }),
		).toBe(true);
	});

	it("returns true for bare -devel versions", () => {
		expect(isDevBuild({ ...baseBuildInfo, version: "v2.32.0-devel" })).toBe(
			true,
		);
	});

	it("returns true for v0.0.0", () => {
		expect(isDevBuild({ ...baseBuildInfo, version: "v0.0.0" })).toBe(true);
	});

	it("returns false for release versions", () => {
		expect(isDevBuild({ ...baseBuildInfo, version: "v2.16.0" })).toBe(false);
	});

	it("returns false for RC versions", () => {
		expect(isDevBuild({ ...baseBuildInfo, version: "v2.32.0-rc.1" })).toBe(
			false,
		);
	});

	it("returns true for combined rc+devel versions", () => {
		expect(
			isDevBuild({
				...baseBuildInfo,
				version: "v2.33.0-rc.1-devel+727ec00f7",
			}),
		).toBe(true);
	});

	it("returns false for empty version", () => {
		expect(isDevBuild({ ...baseBuildInfo, version: "" })).toBe(false);
	});
});

describe("isRcBuild", () => {
	it("returns true for -rc.0 versions", () => {
		expect(isRcBuild({ ...baseBuildInfo, version: "v2.32.0-rc.0" })).toBe(true);
	});

	it("returns true for -rc.1 with build metadata", () => {
		expect(
			isRcBuild({ ...baseBuildInfo, version: "v2.32.0-rc.1+abc123" }),
		).toBe(true);
	});

	it("returns true for higher RC numbers", () => {
		expect(isRcBuild({ ...baseBuildInfo, version: "v2.32.0-rc.12" })).toBe(
			true,
		);
	});

	it("returns false for release versions", () => {
		expect(isRcBuild({ ...baseBuildInfo, version: "v2.16.0" })).toBe(false);
	});

	it("returns false for devel versions", () => {
		expect(
			isRcBuild({ ...baseBuildInfo, version: "v2.16.0-devel+abc123" }),
		).toBe(false);
	});

	it("returns false for empty version", () => {
		expect(isRcBuild({ ...baseBuildInfo, version: "" })).toBe(false);
	});

	it("returns false for versions with rc but no dot", () => {
		expect(isRcBuild({ ...baseBuildInfo, version: "v2.32.0-rc" })).toBe(false);
	});

	it("returns true for combined rc+devel versions", () => {
		expect(
			isRcBuild({
				...baseBuildInfo,
				version: "v2.33.0-rc.1-devel+727ec00f7",
			}),
		).toBe(true);
	});
});
