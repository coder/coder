import { defineConfig } from "@playwright/test";
import { execSync } from "child_process";
import * as path from "path";
import {
  coderMain,
  coderPort,
  coderdPProfPort,
  e2eFakeExperiment1,
  e2eFakeExperiment2,
  gitAuth,
  requireTerraformTests,
} from "./constants";

export const wsEndpoint = process.env.CODER_E2E_WS_ENDPOINT;

// This is where auth cookies are stored!
export const storageState = path.join(__dirname, ".auth.json");

// If running terraform tests, verify the requirements exist in the
// environment.
//
// These execs will throw an error if the status code is non-zero.
// So if both these work, then we can launch terraform provisioners.
let hasTerraform = false;
let hasDocker = false;
try {
  execSync("terraform --version");
  hasTerraform = true;
} catch {
  /* empty */
}

try {
  execSync("docker --version");
  hasDocker = true;
} catch {
  /* empty */
}

if (!hasTerraform || !hasDocker) {
  const msg =
    "Terraform provisioners require docker & terraform binaries to function. \n" +
    (hasTerraform
      ? ""
      : "\tThe `terraform` executable is not present in the runtime environment.\n") +
    (hasDocker
      ? ""
      : "\tThe `docker` executable is not present in the runtime environment.\n");
  throw new Error(msg);
}

const localURL = (port: number, path: string): string => {
  return `http://localhost:${port}${path}`;
};

export default defineConfig({
  projects: [
    {
      name: "testsSetup",
      testMatch: /global.setup\.ts/,
    },
    {
      name: "tests",
      testMatch: /.*\.spec\.ts/,
      dependencies: ["testsSetup"],
      use: { storageState },
      timeout: 50_000,
    },
  ],
  reporter: [["./reporter.ts"]],
  use: {
    baseURL: `http://localhost:${coderPort}`,
    video: "retain-on-failure",
    ...(wsEndpoint
      ? {
          connectOptions: {
            wsEndpoint: wsEndpoint,
          },
        }
      : {
          launchOptions: {
            args: ["--disable-webgl"],
          },
        }),
  },
  webServer: {
    url: `http://localhost:${coderPort}/api/v2/deployment/config`,
    command: [
      `go run -tags embed ${coderMain} server`,
      "--global-config $(mktemp -d -t e2e-XXXXXXXXXX)",
      `--access-url=http://localhost:${coderPort}`,
      `--http-address=0.0.0.0:${coderPort}`,
      "--in-memory",
      "--telemetry=false",
      "--dangerous-disable-rate-limits",
      "--provisioner-daemons 10",
      // TODO: Enable some terraform provisioners
      `--provisioner-types=echo${requireTerraformTests ? ",terraform" : ""}`,
      `--provisioner-daemons=10`,
      "--web-terminal-renderer=dom",
      "--pprof-enable",
    ]
      .filter(Boolean)
      .join(" "),
    env: {
      ...process.env,
      // Otherwise, the runner fails on Mac with: could not determine kind of name for C.uuid_string_t
      CGO_ENABLED: "0",

      // This is the test provider for git auth with devices!
      CODER_GITAUTH_0_ID: gitAuth.deviceProvider,
      CODER_GITAUTH_0_TYPE: "github",
      CODER_GITAUTH_0_CLIENT_ID: "client",
      CODER_GITAUTH_0_CLIENT_SECRET: "secret",
      CODER_GITAUTH_0_DEVICE_FLOW: "true",
      CODER_GITAUTH_0_APP_INSTALL_URL:
        "https://github.com/apps/coder/installations/new",
      CODER_GITAUTH_0_APP_INSTALLATIONS_URL: localURL(
        gitAuth.devicePort,
        gitAuth.installationsPath,
      ),
      CODER_GITAUTH_0_TOKEN_URL: localURL(
        gitAuth.devicePort,
        gitAuth.tokenPath,
      ),
      CODER_GITAUTH_0_DEVICE_CODE_URL: localURL(
        gitAuth.devicePort,
        gitAuth.codePath,
      ),
      CODER_GITAUTH_0_VALIDATE_URL: localURL(
        gitAuth.devicePort,
        gitAuth.validatePath,
      ),

      CODER_GITAUTH_1_ID: gitAuth.webProvider,
      CODER_GITAUTH_1_TYPE: "github",
      CODER_GITAUTH_1_CLIENT_ID: "client",
      CODER_GITAUTH_1_CLIENT_SECRET: "secret",
      CODER_GITAUTH_1_AUTH_URL: localURL(gitAuth.webPort, gitAuth.authPath),
      CODER_GITAUTH_1_TOKEN_URL: localURL(gitAuth.webPort, gitAuth.tokenPath),
      CODER_GITAUTH_1_DEVICE_CODE_URL: localURL(
        gitAuth.webPort,
        gitAuth.codePath,
      ),
      CODER_GITAUTH_1_VALIDATE_URL: localURL(
        gitAuth.webPort,
        gitAuth.validatePath,
      ),
      CODER_PPROF_ADDRESS: "127.0.0.1:" + coderdPProfPort,
      CODER_EXPERIMENTS: e2eFakeExperiment1 + "," + e2eFakeExperiment2,

      // Tests for Deployment / User Authentication / OIDC
      CODER_OIDC_ISSUER_URL: "https://accounts.google.com",
      CODER_OIDC_EMAIL_DOMAIN: "coder.com",
      CODER_OIDC_CLIENT_ID: "1234567890",
      CODER_OIDC_CLIENT_SECRET: "1234567890Secret",
      CODER_OIDC_ALLOW_SIGNUPS: "false",
      CODER_OIDC_SIGN_IN_TEXT: "Hello",
      CODER_OIDC_ICON_URL: "/icon/google.svg",
    },
    reuseExistingServer: false,
  },
});
