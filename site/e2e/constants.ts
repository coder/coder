import * as path from "node:path";

export const coderBinary = path.join(__dirname, "./bin/coder");

// The oldest client and agent versions that Coder still supports. The
// compatibility tests download these release binaries and run them against the
// current server. Changing either value changes which release asset the e2e
// suite fetches, and invalidates the CI cache that stores them.
//
// we no longer support versions prior to Tailnet v2 API support: https://github.com/coder/coder/commit/059e533544a0268acbc8831006b2858ead2f0d8e
export const oldestSupportedCLIVersion = "v2.8.0";
// we no longer support versions w/o DRPC
export const oldestSupportedAgentVersion = "v2.12.1";

// Default port from the server
export const coderPort = process.env.CODER_E2E_PORT
	? Number(process.env.CODER_E2E_PORT)
	: 3111;
export const prometheusPort = 2114;
export const workspaceProxyPort = 3112;

// Use alternate ports in case we're running in a Coder Workspace.
export const agentPProfPort = 6061;
export const coderdPProfPort = 6062;

// The name of the organization that should be used by default when needed.
export const defaultOrganizationName = "coder";
export const defaultOrganizationId = "00000000-0000-0000-0000-000000000000";
export const defaultPassword = "SomeSecurePassword!";

// Credentials for users
export const users = {
	owner: {
		username: "owner",
		password: defaultPassword,
		email: "owner@coder.com",
	},
	templateAdmin: {
		username: "template-admin",
		password: defaultPassword,
		email: "templateadmin@coder.com",
		roles: ["Template Admin"],
	},
	userAdmin: {
		username: "user-admin",
		password: defaultPassword,
		email: "useradmin@coder.com",
		roles: ["User Admin"],
	},
	auditor: {
		username: "auditor",
		password: defaultPassword,
		email: "auditor@coder.com",
		roles: ["Auditor"],
	},
	member: {
		username: "member",
		password: defaultPassword,
		email: "member@coder.com",
	},
} satisfies Record<
	string,
	{ username: string; password: string; email: string; roles?: string[] }
>;

export const gitAuth = {
	deviceProvider: "device",
	webProvider: "web",
	// Keep these below Linux's default ephemeral port range. Otherwise, an
	// outbound connection can claim one and prevent the mock server from binding.
	devicePort: process.env.CODER_E2E_GITAUTH_DEVICE_PORT
		? Number(process.env.CODER_E2E_GITAUTH_DEVICE_PORT)
		: 29515,
	webPort: process.env.CODER_E2E_GITAUTH_WEB_PORT
		? Number(process.env.CODER_E2E_GITAUTH_WEB_PORT)
		: 29516,

	authPath: "/auth",
	tokenPath: "/token",
	codePath: "/code",
	validatePath: "/validate",
	installationsPath: "/installations",
};

/**
 * Will make the tests fail if set to `true` and a license was not provided.
 */
export const premiumTestsRequired = Boolean(
	process.env.CODER_E2E_REQUIRE_PREMIUM_TESTS,
);

export const license = process.env.CODER_E2E_LICENSE ?? "";

// Disabling terraform tests is optional for environments without Docker + Terraform.
// By default, we opt into these tests.
export const requireTerraformTests = !process.env.CODER_E2E_DISABLE_TERRAFORM;

// Fake experiments to verify that site presents them as enabled.
export const e2eFakeExperiment1 = "e2e-fake-experiment-1";
export const e2eFakeExperiment2 = "e2e-fake-experiment-2";
