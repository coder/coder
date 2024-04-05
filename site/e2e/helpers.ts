import { type BrowserContext, expect, type Page, test } from "@playwright/test";
import axios from "axios";
import { type ChildProcess, exec, spawn } from "child_process";
import { randomUUID } from "crypto";
import express from "express";
import capitalize from "lodash/capitalize";
import path from "path";
import * as ssh from "ssh2";
import { Duplex } from "stream";
import type {
  WorkspaceBuildParameter,
  UpdateTemplateMeta,
} from "api/typesGenerated";
import { TarWriter } from "utils/tar";
import {
  agentPProfPort,
  coderMain,
  coderPort,
  enterpriseLicense,
  prometheusPort,
} from "./constants";
import {
  Agent,
  type App,
  AppSharingLevel,
  type ParseComplete,
  type PlanComplete,
  type ApplyComplete,
  type Resource,
  Response,
  type RichParameter,
} from "./provisionerGenerated";

// requiresEnterpriseLicense will skip the test if we're not running with an enterprise license
export function requiresEnterpriseLicense() {
  test.skip(!enterpriseLicense);
}

// createWorkspace creates a workspace for a template.
// It does not wait for it to be running, but it does navigate to the page.
export const createWorkspace = async (
  page: Page,
  templateName: string,
  richParameters: RichParameter[] = [],
  buildParameters: WorkspaceBuildParameter[] = [],
): Promise<string> => {
  await page.goto("/templates/" + templateName + "/workspace", {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL("/templates/" + templateName + "/workspace");

  const name = randomName();
  await page.getByLabel("name").fill(name);

  await fillParameters(page, richParameters, buildParameters);
  await page.getByTestId("form-submit").click();

  await expect(page).toHaveURL("/@admin/" + name);

  await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
    state: "visible",
  });
  return name;
};

export const verifyParameters = async (
  page: Page,
  workspaceName: string,
  richParameters: RichParameter[],
  expectedBuildParameters: WorkspaceBuildParameter[],
) => {
  await page.goto("/@admin/" + workspaceName + "/settings/parameters", {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(
    "/@admin/" + workspaceName + "/settings/parameters",
  );

  for (const buildParameter of expectedBuildParameters) {
    const richParameter = richParameters.find(
      (richParam) => richParam.name === buildParameter.name,
    );
    if (!richParameter) {
      throw new Error(
        "build parameter is expected to be present in rich parameter schema",
      );
    }

    const parameterLabel = await page.waitForSelector(
      "[data-testid='parameter-field-" + richParameter.name + "']",
      { state: "visible" },
    );

    const muiDisabled = richParameter.mutable ? "" : ".Mui-disabled";

    if (richParameter.type === "bool") {
      const parameterField = await parameterLabel.waitForSelector(
        "[data-testid='parameter-field-bool'] .MuiRadio-root.Mui-checked" +
          muiDisabled +
          " input",
      );
      const value = await parameterField.inputValue();
      expect(value).toEqual(buildParameter.value);
    } else if (richParameter.options.length > 0) {
      const parameterField = await parameterLabel.waitForSelector(
        "[data-testid='parameter-field-options'] .MuiRadio-root.Mui-checked" +
          muiDisabled +
          " input",
      );
      const value = await parameterField.inputValue();
      expect(value).toEqual(buildParameter.value);
    } else if (richParameter.type === "list(string)") {
      throw new Error("not implemented yet"); // FIXME
    } else {
      // text or number
      const parameterField = await parameterLabel.waitForSelector(
        "[data-testid='parameter-field-text'] input" + muiDisabled,
      );
      const value = await parameterField.inputValue();
      expect(value).toEqual(buildParameter.value);
    }
  }
};

// createTemplate navigates to the /templates/new page and uploads a template
// with the resources provided in the responses argument.
export const createTemplate = async (
  page: Page,
  responses?: EchoProvisionerResponses,
): Promise<string> => {
  // Required to have templates submit their provisioner type as echo!
  await page.addInitScript({
    content: "window.playwright = true",
  });

  await page.goto("/templates/new", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL("/templates/new");

  await page.getByTestId("file-upload").setInputFiles({
    buffer: await createTemplateVersionTar(responses),
    mimeType: "application/x-tar",
    name: "template.tar",
  });
  const name = randomName();
  await page.getByLabel("Name *").fill(name);
  await page.getByTestId("form-submit").click();
  await expect(page).toHaveURL(`/templates/${name}/files`, {
    timeout: 30000,
  });
  return name;
};

// createGroup navigates to the /groups/create page and creates a group with a
// random name.
export const createGroup = async (page: Page): Promise<string> => {
  await page.goto("/groups/create", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL("/groups/create");

  const name = randomName();
  await page.getByLabel("Name", { exact: true }).fill(name);
  await page.getByTestId("form-submit").click();
  await expect(page).toHaveURL(
    /\/groups\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
  );
  return name;
};

// sshIntoWorkspace spawns a Coder SSH process and a client connected to it.
export const sshIntoWorkspace = async (
  page: Page,
  workspace: string,
  binaryPath = "go",
  binaryArgs: string[] = [],
): Promise<ssh.Client> => {
  if (binaryPath === "go") {
    binaryArgs = ["run", coderMain];
  }
  const sessionToken = await findSessionToken(page);
  return new Promise<ssh.Client>((resolve, reject) => {
    const cp = spawn(binaryPath, [...binaryArgs, "ssh", "--stdio", workspace], {
      env: {
        ...process.env,
        CODER_SESSION_TOKEN: sessionToken,
        CODER_URL: `http://localhost:${coderPort}`,
      },
    });
    cp.on("error", (err) => reject(err));
    const proxyStream = new Duplex({
      read: (size) => {
        return cp.stdout.read(Math.min(size, cp.stdout.readableLength));
      },
      write: cp.stdin.write.bind(cp.stdin),
    });
    // eslint-disable-next-line no-console -- Helpful for debugging
    cp.stderr.on("data", (data) => console.log(data.toString()));
    cp.stdout.on("readable", (...args) => {
      proxyStream.emit("readable", ...args);
      if (cp.stdout.readableLength > 0) {
        proxyStream.emit("data", cp.stdout.read());
      }
    });
    const client = new ssh.Client();
    client.connect({
      sock: proxyStream,
      username: "coder",
    });
    client.on("error", (err) => reject(err));
    client.on("ready", () => {
      resolve(client);
    });
  });
};

export const stopWorkspace = async (page: Page, workspaceName: string) => {
  await page.goto("/@admin/" + workspaceName, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL("/@admin/" + workspaceName);

  await page.getByTestId("workspace-stop-button").click();

  await page.waitForSelector("*[data-testid='build-status'] >> text=Stopped", {
    state: "visible",
  });
};

export const buildWorkspaceWithParameters = async (
  page: Page,
  workspaceName: string,
  richParameters: RichParameter[] = [],
  buildParameters: WorkspaceBuildParameter[] = [],
  confirm: boolean = false,
) => {
  await page.goto("/@admin/" + workspaceName, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL("/@admin/" + workspaceName);

  await page.getByTestId("build-parameters-button").click();

  await fillParameters(page, richParameters, buildParameters);
  await page.getByTestId("build-parameters-submit").click();
  if (confirm) {
    await page.getByTestId("confirm-button").click();
  }

  await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
    state: "visible",
  });
};

// startAgent runs the coder agent with the provided token.
// It awaits the agent to be ready before returning.
export const startAgent = async (
  page: Page,
  token: string,
): Promise<ChildProcess> => {
  return startAgentWithCommand(page, token, "go", "run", coderMain);
};

// downloadCoderVersion downloads the version provided into a temporary dir and
// caches it so subsequent calls are fast.
export const downloadCoderVersion = async (
  version: string,
): Promise<string> => {
  if (version.startsWith("v")) {
    version = version.slice(1);
  }

  const binaryName = "coder-e2e-" + version;
  const tempDir = "/tmp/coder-e2e-cache";
  // The install script adds `./bin` automatically to the path :shrug:
  const binaryPath = path.join(tempDir, "bin", binaryName);

  const exists = await new Promise<boolean>((resolve) => {
    const cp = spawn(binaryPath, ["version"]);
    cp.on("close", (code) => {
      resolve(code === 0);
    });
    cp.on("error", () => resolve(false));
  });
  if (exists) {
    return binaryPath;
  }

  // Run our official install script to install the binary
  await new Promise<void>((resolve, reject) => {
    const cp = spawn(
      path.join(__dirname, "../../install.sh"),
      [
        "--version",
        version,
        "--method",
        "standalone",
        "--prefix",
        tempDir,
        "--binary-name",
        binaryName,
      ],
      {
        env: {
          ...process.env,
          XDG_CACHE_HOME: "/tmp/coder-e2e-cache",
          TRACE: "1", // tells install.sh to `set -x`, helpful if something goes wrong
        },
      },
    );
    // eslint-disable-next-line no-console -- Needed for debugging
    cp.stderr.on("data", (data) => console.error(data.toString()));
    // eslint-disable-next-line no-console -- Needed for debugging
    cp.stdout.on("data", (data) => console.log(data.toString()));
    cp.on("close", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error("install.sh failed with code " + code));
      }
    });
  });
  return binaryPath;
};

export const startAgentWithCommand = async (
  page: Page,
  token: string,
  command: string,
  ...args: string[]
): Promise<ChildProcess> => {
  const cp = spawn(command, [...args, "agent", "--no-reap"], {
    env: {
      ...process.env,
      CODER_AGENT_URL: `http://localhost:${coderPort}`,
      CODER_AGENT_TOKEN: token,
      CODER_AGENT_PPROF_ADDRESS: "127.0.0.1:" + agentPProfPort,
      CODER_AGENT_PROMETHEUS_ADDRESS: "127.0.0.1:" + prometheusPort,
    },
  });
  cp.stdout.on("data", (data: Buffer) => {
    // eslint-disable-next-line no-console -- Log agent activity
    console.log(
      `[agent] [stdout] [onData] ${data.toString().replace(/\n$/g, "")}`,
    );
  });
  cp.stderr.on("data", (data: Buffer) => {
    // eslint-disable-next-line no-console -- Log agent activity
    console.log(
      `[agent] [stderr] [onData] ${data.toString().replace(/\n$/g, "")}`,
    );
  });

  await page.getByTestId("agent-status-ready").waitFor({ state: "visible" });
  return cp;
};

export const stopAgent = async (cp: ChildProcess, goRun: boolean = true) => {
  // When the web server is started with `go run`, it spawns a child process with coder server.
  // `pkill -P` terminates child processes belonging the same group as `go run`.
  // The command `kill` is used to terminate a web server started as a standalone binary.
  exec(goRun ? `pkill -P ${cp.pid}` : `kill ${cp.pid}`, (error) => {
    if (error) {
      throw new Error(`exec error: ${JSON.stringify(error)}`);
    }
  });
  await waitUntilUrlIsNotResponding("http://localhost:" + prometheusPort);
};

const waitUntilUrlIsNotResponding = async (url: string) => {
  const maxRetries = 30;
  const retryIntervalMs = 1000;
  let retries = 0;

  while (retries < maxRetries) {
    try {
      await axios.get(url);
    } catch (error) {
      return;
    }

    retries++;
    await new Promise((resolve) => setTimeout(resolve, retryIntervalMs));
  }
  throw new Error(
    `URL ${url} is still responding after ${maxRetries * retryIntervalMs}ms`,
  );
};

// Allows users to more easily define properties they want for agents and resources!
type RecursivePartial<T> = {
  [P in keyof T]?: T[P] extends (infer U)[]
    ? RecursivePartial<U>[]
    : T[P] extends object | undefined
      ? RecursivePartial<T[P]>
      : T[P];
};

interface EchoProvisionerResponses {
  // parse is for observing any Terraform variables
  parse?: RecursivePartial<Response>[];
  // plan occurs when the template is imported
  plan?: RecursivePartial<Response>[];
  // apply occurs when the workspace is built
  apply?: RecursivePartial<Response>[];
}

// createTemplateVersionTar consumes a series of echo provisioner protobufs and
// converts it into an uploadable tar file.
const createTemplateVersionTar = async (
  responses?: EchoProvisionerResponses,
): Promise<Buffer> => {
  if (!responses) {
    responses = {};
  }
  if (!responses.parse) {
    responses.parse = [
      {
        parse: {},
      },
    ];
  }
  if (!responses.apply) {
    responses.apply = [
      {
        apply: {},
      },
    ];
  }
  if (!responses.plan) {
    responses.plan = responses.apply.map((response) => {
      if (response.log) {
        return response;
      }
      return {
        plan: {
          error: response.apply?.error ?? "",
          resources: response.apply?.resources ?? [],
          parameters: response.apply?.parameters ?? [],
          externalAuthProviders: response.apply?.externalAuthProviders ?? [],
        },
      };
    });
  }

  const tar = new TarWriter();
  responses.parse.forEach((response, index) => {
    response.parse = {
      templateVariables: [],
      error: "",
      readme: new Uint8Array(),
      ...response.parse,
    } as ParseComplete;
    tar.addFile(
      `${index}.parse.protobuf`,
      Response.encode(response as Response).finish(),
    );
  });

  const fillResource = (resource: RecursivePartial<Resource>) => {
    if (resource.agents) {
      resource.agents = resource.agents?.map(
        (agent: RecursivePartial<Agent>) => {
          if (agent.apps) {
            agent.apps = agent.apps.map((app) => {
              return {
                command: "",
                displayName: "example",
                external: false,
                icon: "",
                sharingLevel: AppSharingLevel.PUBLIC,
                slug: "example",
                subdomain: false,
                url: "",
                ...app,
              } as App;
            });
          }
          const agentResource = {
            apps: [],
            architecture: "amd64",
            connectionTimeoutSeconds: 300,
            directory: "",
            env: {},
            id: randomUUID(),
            metadata: [],
            extraEnvs: [],
            scripts: [],
            motdFile: "",
            name: "dev",
            operatingSystem: "linux",
            shutdownScript: "",
            shutdownScriptTimeoutSeconds: 0,
            startupScript: "",
            startupScriptBehavior: "",
            startupScriptTimeoutSeconds: 300,
            troubleshootingUrl: "",
            token: randomUUID(),
            ...agent,
          } as Agent;

          try {
            Agent.encode(agentResource);
          } catch (e) {
            let m = `Error: agentResource encode failed, missing defaults?`;
            if (e instanceof Error) {
              if (!e.stack?.includes(e.message)) {
                m += `\n${e.name}: ${e.message}`;
              }
              m += `\n${e.stack}`;
            } else {
              m += `\n${e}`;
            }
            throw new Error(m);
          }

          return agentResource;
        },
      );
    }
    return {
      agents: [],
      dailyCost: 0,
      hide: false,
      icon: "",
      instanceType: "",
      metadata: [],
      name: "dev",
      type: "echo",
      ...resource,
    } as Resource;
  };

  responses.apply.forEach((response, index) => {
    response.apply = {
      error: "",
      state: new Uint8Array(),
      resources: [],
      parameters: [],
      externalAuthProviders: [],
      ...response.apply,
    } as ApplyComplete;
    response.apply.resources = response.apply.resources?.map(fillResource);

    tar.addFile(
      `${index}.apply.protobuf`,
      Response.encode(response as Response).finish(),
    );
  });
  responses.plan.forEach((response, index) => {
    response.plan = {
      error: "",
      resources: [],
      parameters: [],
      externalAuthProviders: [],
      ...response.plan,
    } as PlanComplete;
    response.plan.resources = response.plan.resources?.map(fillResource);

    tar.addFile(
      `${index}.plan.protobuf`,
      Response.encode(response as Response).finish(),
    );
  });
  const tarFile = await tar.write();
  return Buffer.from(
    tarFile instanceof Blob ? await tarFile.arrayBuffer() : tarFile,
  );
};

export const randomName = () => {
  return randomUUID().slice(0, 8);
};

// Awaiter is a helper that allows you to wait for a callback to be called.
// It is useful for waiting for events to occur.
export class Awaiter {
  private promise: Promise<void>;
  private callback?: () => void;

  constructor() {
    this.promise = new Promise((r) => (this.callback = r));
  }

  public done(): void {
    if (this.callback) {
      this.callback();
    } else {
      this.promise = Promise.resolve();
    }
  }

  public wait(): Promise<void> {
    return this.promise;
  }
}

export const createServer = async (
  port: number,
): Promise<ReturnType<typeof express>> => {
  const e = express();
  // We need to specify the local IP address as the web server
  // tends to fail with IPv6 related error:
  // listen EADDRINUSE: address already in use :::50516
  await new Promise<void>((r) => e.listen(port, "0.0.0.0", r));
  return e;
};

export const findSessionToken = async (page: Page): Promise<string> => {
  const cookies = await page.context().cookies();
  const sessionCookie = cookies.find((c) => c.name === "coder_session_token");
  if (!sessionCookie) {
    throw new Error("session token not found");
  }
  return sessionCookie.value;
};

export const echoResponsesWithParameters = (
  richParameters: RichParameter[],
): EchoProvisionerResponses => {
  return {
    parse: [
      {
        parse: {},
      },
    ],
    plan: [
      {
        plan: {
          parameters: richParameters,
        },
      },
    ],
    apply: [
      {
        apply: {
          resources: [
            {
              name: "example",
            },
          ],
        },
      },
    ],
  };
};

export const fillParameters = async (
  page: Page,
  richParameters: RichParameter[] = [],
  buildParameters: WorkspaceBuildParameter[] = [],
) => {
  for (const buildParameter of buildParameters) {
    const richParameter = richParameters.find(
      (richParam) => richParam.name === buildParameter.name,
    );
    if (!richParameter) {
      throw new Error(
        "build parameter is expected to be present in rich parameter schema",
      );
    }

    const parameterLabel = await page.waitForSelector(
      "[data-testid='parameter-field-" + richParameter.name + "']",
      { state: "visible" },
    );

    if (richParameter.type === "bool") {
      const parameterField = await parameterLabel.waitForSelector(
        "[data-testid='parameter-field-bool'] .MuiRadio-root input[value='" +
          buildParameter.value +
          "']",
      );
      await parameterField.click();
    } else if (richParameter.options.length > 0) {
      const parameterField = await parameterLabel.waitForSelector(
        "[data-testid='parameter-field-options'] .MuiRadio-root input[value='" +
          buildParameter.value +
          "']",
      );
      await parameterField.click();
    } else if (richParameter.type === "list(string)") {
      throw new Error("not implemented yet"); // FIXME
    } else {
      // text or number
      const parameterField = await parameterLabel.waitForSelector(
        "[data-testid='parameter-field-text'] input",
      );
      await parameterField.fill(buildParameter.value);
    }
  }
};

export const updateTemplate = async (
  page: Page,
  templateName: string,
  responses?: EchoProvisionerResponses,
) => {
  const tarball = await createTemplateVersionTar(responses);

  const sessionToken = await findSessionToken(page);
  const child = spawn(
    "go",
    [
      "run",
      coderMain,
      "templates",
      "push",
      "--test.provisioner",
      "echo",
      "-y",
      "-d",
      "-",
      templateName,
    ],
    {
      env: {
        ...process.env,
        CODER_SESSION_TOKEN: sessionToken,
        CODER_URL: `http://localhost:${coderPort}`,
      },
    },
  );

  const uploaded = new Awaiter();
  child.on("exit", (code) => {
    if (code === 0) {
      uploaded.done();
      return;
    }

    throw new Error(`coder templates push failed with code ${code}`);
  });

  child.stdin.write(tarball);
  child.stdin.end();

  await uploaded.wait();
};

export const updateTemplateSettings = async (
  page: Page,
  templateName: string,
  templateSettingValues: Pick<
    UpdateTemplateMeta,
    "name" | "display_name" | "description" | "deprecation_message"
  >,
) => {
  await page.goto(`/templates/${templateName}/settings`, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(`/templates/${templateName}/settings`);

  for (const [key, value] of Object.entries(templateSettingValues)) {
    // Skip max_port_share_level for now since the frontend is not yet able to handle it
    if (key === "max_port_share_level") {
      continue;
    }
    const labelText = capitalize(key).replace("_", " ");
    await page.getByLabel(labelText, { exact: true }).fill(value);
  }

  await page.getByTestId("form-submit").click();

  const name = templateSettingValues.name ?? templateName;
  await expect(page).toHaveURL(`/templates/${name}`);
};

export const updateWorkspace = async (
  page: Page,
  workspaceName: string,
  richParameters: RichParameter[] = [],
  buildParameters: WorkspaceBuildParameter[] = [],
) => {
  await page.goto("/@admin/" + workspaceName, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL("/@admin/" + workspaceName);

  await page.getByTestId("workspace-update-button").click();
  await page.getByTestId("confirm-button").click();

  await fillParameters(page, richParameters, buildParameters);
  await page.getByTestId("form-submit").click();

  await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
    state: "visible",
  });
};

export const updateWorkspaceParameters = async (
  page: Page,
  workspaceName: string,
  richParameters: RichParameter[] = [],
  buildParameters: WorkspaceBuildParameter[] = [],
) => {
  await page.goto("/@admin/" + workspaceName + "/settings/parameters", {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(
    "/@admin/" + workspaceName + "/settings/parameters",
  );

  await fillParameters(page, richParameters, buildParameters);
  await page.getByTestId("form-submit").click();

  await page.waitForSelector("*[data-testid='build-status'] >> text=Running", {
    state: "visible",
  });
};

export async function openTerminalWindow(
  page: Page,
  context: BrowserContext,
  workspaceName: string,
): Promise<Page> {
  // Wait for the web terminal to open in a new tab
  const pagePromise = context.waitForEvent("page");
  await page.getByTestId("terminal").click();
  const terminal = await pagePromise;
  await terminal.waitForLoadState("domcontentloaded");

  // Specify that the shell should be `bash`, to prevent inheriting a shell that
  // isn't POSIX compatible, such as Fish.
  const commandQuery = `?command=${encodeURIComponent("/usr/bin/env bash")}`;
  await expect(terminal).toHaveURL(`/@admin/${workspaceName}.dev/terminal`);
  await terminal.goto(`/@admin/${workspaceName}.dev/terminal${commandQuery}`);

  return terminal;
}
