import { createAppLinkHref } from "./apps";
import {
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceApp,
} from "testHelpers/entities";

describe("create app link", () => {
  it("with external URL", () => {
    const externalURL = "https://external-url.tld";
    const href = createAppLinkHref(
      "http:",
      "/path-base",
      "*.apps-host.tld",
      "app-slug",
      "username",
      MockWorkspace,
      MockWorkspaceAgent,
      {
        ...MockWorkspaceApp,
        external: true,
        url: externalURL,
      },
    );
    expect(href).toBe(externalURL);
  });

  it("without subdomain", () => {
    const href = createAppLinkHref(
      "http:",
      "/path-base",
      "*.apps-host.tld",
      "app-slug",
      "username",
      MockWorkspace,
      MockWorkspaceAgent,
      {
        ...MockWorkspaceApp,
        subdomain: false,
      },
    );
    expect(href).toBe(
      "/path-base/@username/Test-Workspace.a-workspace-agent/apps/app-slug/",
    );
  });

  it("with command", () => {
    const href = createAppLinkHref(
      "https:",
      "/path-base",
      "*.apps-host.tld",
      "app-slug",
      "username",
      MockWorkspace,
      MockWorkspaceAgent,
      {
        ...MockWorkspaceApp,
        command: "ls -la",
      },
    );
    expect(href).toBe(
      "/@username/Test-Workspace.a-workspace-agent/terminal?command=ls%20-la",
    );
  });

  it("with subdomain", () => {
    const href = createAppLinkHref(
      "ftps:",
      "/path-base",
      "*.apps-host.tld",
      "app-slug",
      "username",
      MockWorkspace,
      MockWorkspaceAgent,
      {
        ...MockWorkspaceApp,
        subdomain: true,
        subdomain_name: "hellocoder",
      },
    );
    expect(href).toBe("ftps://hellocoder.apps-host.tld/");
  });

  it("with subdomain, but not apps host", () => {
    const href = createAppLinkHref(
      "ftps:",
      "/path-base",
      "",
      "app-slug",
      "username",
      MockWorkspace,
      MockWorkspaceAgent,
      {
        ...MockWorkspaceApp,
        subdomain: true,
        subdomain_name: "hellocoder",
      },
    );
    expect(href).toBe(
      "/path-base/@username/Test-Workspace.a-workspace-agent/apps/app-slug/",
    );
  });
});
