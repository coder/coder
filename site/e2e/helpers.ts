import { expect, Page } from "@playwright/test"
import { spawn } from "child_process"
import { randomUUID } from "crypto"
import path from "path"
import express from "express"
import { TarWriter } from "utils/tar"
import {
  Agent,
  App,
  AppSharingLevel,
  Parse_Complete,
  Parse_Response,
  Provision_Complete,
  Provision_Response,
  Resource,
} from "./provisionerGenerated"
import { port } from "./playwright.config"
import * as ssh from "ssh2"
import { Duplex } from "stream"

// createWorkspace creates a workspace for a template.
// It does not wait for it to be running, but it does navigate to the page.
export const createWorkspace = async (
  page: Page,
  templateName: string,
): Promise<string> => {
  await page.goto("/templates/" + templateName + "/workspace", {
    waitUntil: "networkidle",
  })
  const name = randomName()
  await page.getByLabel("name").fill(name)
  await page.getByTestId("form-submit").click()

  await expect(page).toHaveURL("/@admin/" + name)
  await page.getByTestId("build-status").isVisible()
  return name
}

// createTemplate navigates to the /templates/new page and uploads a template
// with the resources provided in the responses argument.
export const createTemplate = async (
  page: Page,
  responses?: EchoProvisionerResponses,
): Promise<string> => {
  // Required to have templates submit their provisioner type as echo!
  await page.addInitScript({
    content: "window.playwright = true",
  })
  await page.goto("/templates/new", { waitUntil: "networkidle" })
  await page.getByTestId("file-upload").setInputFiles({
    buffer: await createTemplateVersionTar(responses),
    mimeType: "application/x-tar",
    name: "template.tar",
  })
  const name = randomName()
  await page.getByLabel("Name *").fill(name)
  await page.getByTestId("form-submit").click()
  await expect(page).toHaveURL("/templates/" + name, {
    timeout: 30000,
  })
  return name
}

// sshIntoWorkspace spawns a Coder SSH process and a client connected to it.
export const sshIntoWorkspace = async (
  page: Page,
  workspace: string,
): Promise<ssh.Client> => {
  const sessionToken = await findSessionToken(page)
  return new Promise<ssh.Client>((resolve, reject) => {
    const cp = spawn(
      "go",
      ["run", coderMainPath(), "ssh", "--stdio", workspace],
      {
        env: {
          ...process.env,
          CODER_SESSION_TOKEN: sessionToken,
          CODER_URL: "http://localhost:3000",
        },
      },
    )
    cp.on("error", (err) => reject(err))
    const proxyStream = new Duplex({
      read: (size) => {
        return cp.stdout.read(Math.min(size, cp.stdout.readableLength))
      },
      write: cp.stdin.write.bind(cp.stdin),
    })
    // eslint-disable-next-line no-console -- Helpful for debugging
    cp.stderr.on("data", (data) => console.log(data.toString()))
    cp.stdout.on("readable", (...args) => {
      proxyStream.emit("readable", ...args)
      if (cp.stdout.readableLength > 0) {
        proxyStream.emit("data", cp.stdout.read())
      }
    })
    const client = new ssh.Client()
    client.connect({
      sock: proxyStream,
      username: "coder",
    })
    client.on("error", (err) => reject(err))
    client.on("ready", () => {
      resolve(client)
    })
  })
}

// startAgent runs the coder agent with the provided token.
// It awaits the agent to be ready before returning.
export const startAgent = async (page: Page, token: string): Promise<void> => {
  return startAgentWithCommand(page, token, "go", "run", coderMainPath())
}

// downloadCoderVersion downloads the version provided into a temporary dir and
// caches it so subsequent calls are fast.
export const downloadCoderVersion = async (
  version: string,
): Promise<string> => {
  if (version.startsWith("v")) {
    version = version.slice(1)
  }

  const binaryName = "coder-e2e-" + version
  const tempDir = "/tmp"
  // The install script adds `./bin` automatically to the path :shrug:
  const binaryPath = path.join(tempDir, "bin", binaryName)

  const exists = await new Promise<boolean>((resolve) => {
    const cp = spawn(binaryPath, ["version"])
    cp.on("close", (code) => {
      resolve(code === 0)
    })
    cp.on("error", () => resolve(false))
  })
  if (exists) {
    return binaryPath
  }

  // Runs our public install script using our options to
  // install the binary!
  await new Promise<void>((resolve, reject) => {
    const cp = spawn("sh", [
      "-c",
      [
        "curl",
        "-L",
        "https://coder.com/install.sh",
        "|",
        "sh",
        "-s",
        "--",
        "--version",
        version,
        "--method",
        "standalone",
        "--prefix",
        tempDir,
        "--binary-name",
        binaryName,
      ].join(" "),
    ])
    // eslint-disable-next-line no-console -- Needed for debugging
    cp.stderr.on("data", (data) => console.log(data.toString()))
    cp.on("close", (code) => {
      if (code === 0) {
        resolve()
      } else {
        reject(new Error("curl failed with code " + code))
      }
    })
  })
  return binaryPath
}

export const startAgentWithCommand = async (
  page: Page,
  token: string,
  command: string,
  ...args: string[]
): Promise<void> => {
  const cp = spawn(command, [...args, "agent", "--no-reap"], {
    env: {
      ...process.env,
      CODER_AGENT_URL: "http://localhost:" + port,
      CODER_AGENT_TOKEN: token,
    },
  })
  let buffer = Buffer.of()
  cp.stderr.on("data", (data: Buffer) => {
    buffer = Buffer.concat([buffer, data])
  })
  try {
    await page.getByTestId("agent-status-ready").isVisible()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- The error is a string
  } catch (ex: any) {
    throw new Error(ex.toString() + "\n" + buffer.toString())
  }
}

const coderMainPath = (): string => {
  return path.join(
    __dirname,
    "..",
    "..",
    "enterprise",
    "cmd",
    "coder",
    "main.go",
  )
}

// Allows users to more easily define properties they want for agents and resources!
type RecursivePartial<T> = {
  [P in keyof T]?: T[P] extends (infer U)[]
    ? RecursivePartial<U>[]
    : T[P] extends object | undefined
    ? RecursivePartial<T[P]>
    : T[P]
}

interface EchoProvisionerResponses {
  // parse is for observing any Terraform variables
  parse?: RecursivePartial<Parse_Response>[]
  // plan occurs when the template is imported
  plan?: RecursivePartial<Provision_Response>[]
  // apply occurs when the workspace is built
  apply?: RecursivePartial<Provision_Response>[]
}

// createTemplateVersionTar consumes a series of echo provisioner protobufs and
// converts it into an uploadable tar file.
const createTemplateVersionTar = async (
  responses?: EchoProvisionerResponses,
): Promise<Buffer> => {
  if (!responses) {
    responses = {}
  }
  if (!responses.parse) {
    responses.parse = [{}]
  }
  if (!responses.apply) {
    responses.apply = [{}]
  }
  if (!responses.plan) {
    responses.plan = responses.apply
  }

  const tar = new TarWriter()
  responses.parse.forEach((response, index) => {
    response.complete = {
      templateVariables: [],
      ...response.complete,
    } as Parse_Complete
    tar.addFile(
      `${index}.parse.protobuf`,
      Parse_Response.encode(response as Parse_Response).finish(),
    )
  })

  const fillProvisionResponse = (
    response: RecursivePartial<Provision_Response>,
  ) => {
    response.complete = {
      error: "",
      state: new Uint8Array(),
      resources: [],
      parameters: [],
      gitAuthProviders: [],
      plan: new Uint8Array(),
      ...response.complete,
    } as Provision_Complete
    response.complete.resources = response.complete.resources?.map(
      (resource) => {
        if (resource.agents) {
          resource.agents = resource.agents?.map((agent) => {
            if (agent.apps) {
              agent.apps = agent.apps?.map((app) => {
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
                } as App
              })
            }
            return {
              apps: [],
              architecture: "amd64",
              connectionTimeoutSeconds: 300,
              directory: "",
              env: {},
              id: randomUUID(),
              metadata: [],
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
            } as Agent
          })
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
        } as Resource
      },
    )
  }

  responses.apply.forEach((response, index) => {
    fillProvisionResponse(response)

    tar.addFile(
      `${index}.provision.apply.protobuf`,
      Provision_Response.encode(response as Provision_Response).finish(),
    )
  })
  responses.plan.forEach((response, index) => {
    fillProvisionResponse(response)

    tar.addFile(
      `${index}.provision.plan.protobuf`,
      Provision_Response.encode(response as Provision_Response).finish(),
    )
  })
  const tarFile = await tar.write()
  return Buffer.from(
    tarFile instanceof Blob ? await tarFile.arrayBuffer() : tarFile,
  )
}

const randomName = () => {
  return randomUUID().slice(0, 8)
}

// Awaiter is a helper that allows you to wait for a callback to be called.
// It is useful for waiting for events to occur.
export class Awaiter {
  private promise: Promise<void>
  private callback?: () => void

  constructor() {
    this.promise = new Promise((r) => (this.callback = r))
  }

  public done(): void {
    if (this.callback) {
      this.callback()
    } else {
      this.promise = Promise.resolve()
    }
  }

  public wait(): Promise<void> {
    return this.promise
  }
}

export const createServer = async (
  port: number,
): Promise<ReturnType<typeof express>> => {
  const e = express()
  await new Promise<void>((r) => e.listen(port, r))
  return e
}

const findSessionToken = async (page: Page): Promise<string> => {
  const cookies = await page.context().cookies()
  const sessionCookie = cookies.find((c) => c.name === "coder_session_token")
  if (!sessionCookie) {
    throw new Error("session token not found")
  }
  return sessionCookie.value
}
