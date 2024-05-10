import * as path from "path";

export const coderMain = path.join(__dirname, "../../enterprise/cmd/coder");

// Default port from the server
export const coderPort = process.env.CODER_E2E_PORT
  ? Number(process.env.CODER_E2E_PORT)
  : 3111;
export const prometheusPort = 2114;
export const workspaceProxyPort = 3112;

// Use alternate ports in case we're running in a Coder Workspace.
export const agentPProfPort = 6061;
export const coderdPProfPort = 6062;

// Credentials for the first user
export const username = "admin";
export const password = "SomeSecurePassword!";
export const email = "admin@coder.com";

export const gitAuth = {
  deviceProvider: "device",
  webProvider: "web",
  // These ports need to be hardcoded so that they can be
  // used in `playwright.config.ts` to set the environment
  // variables for the server.
  devicePort: 50515,
  webPort: 50516,

  authPath: "/auth",
  tokenPath: "/token",
  codePath: "/code",
  validatePath: "/validate",
  installationsPath: "/installations",
};

export const requireEnterpriseTests = Boolean(
  process.env.CODER_E2E_REQUIRE_ENTERPRISE_TESTS,
);
export const enterpriseLicense = process.env.CODER_E2E_ENTERPRISE_LICENSE ?? "";

// Disabling terraform tests is optional for environments without Docker + Terraform.
// By default, we opt into these tests.
export const requireTerraformTests = !process.env.CODER_E2E_DISABLE_TERRAFORM;

// Fake experiments to verify that site presents them as enabled.
export const e2eFakeExperiment1 = "e2e-fake-experiment-1";
export const e2eFakeExperiment2 = "e2e-fake-experiment-2";
