import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { MockPrimaryWorkspaceProxy, MockUser } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { Language as navLanguage, NavbarView } from "./NavbarView";

const proxyContextValue: ProxyContextValue = {
  proxy: {
    preferredPathAppURL: "",
    preferredWildcardHostname: "",
    proxy: MockPrimaryWorkspaceProxy,
  },
  isLoading: false,
  isFetched: true,
  setProxy: jest.fn(),
  clearProxy: jest.fn(),
  refetchProxyLatencies: jest.fn(),
  proxyLatencies: {},
};

describe("NavbarView", () => {
  const noop = jest.fn();

  it("workspaces nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewDeployment
        canViewOrganizations
        canViewAllUsers
        canViewHealth
        canViewAuditLog
      />,
    );
    const workspacesLink = await screen.findByText(navLanguage.workspaces);
    expect((workspacesLink as HTMLAnchorElement).href).toContain("/workspaces");
  });

  it("templates nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewDeployment
        canViewOrganizations
        canViewAllUsers
        canViewHealth
        canViewAuditLog
      />,
    );
    const templatesLink = await screen.findByText(navLanguage.templates);
    expect((templatesLink as HTMLAnchorElement).href).toContain("/templates");
  });

  it("users nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewDeployment
        canViewOrganizations
        canViewAllUsers
        canViewHealth
        canViewAuditLog
      />,
    );
    const deploymentMenu = await screen.findByText("Deployment");
    await userEvent.click(deploymentMenu);
    const userLink = await screen.findByText(navLanguage.users);
    expect((userLink as HTMLAnchorElement).href).toContain("/users");
  });

  it("audit nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewDeployment
        canViewOrganizations
        canViewAllUsers
        canViewHealth
        canViewAuditLog
      />,
    );
    const deploymentMenu = await screen.findByText("Deployment");
    await userEvent.click(deploymentMenu);
    const auditLink = await screen.findByText(navLanguage.audit);
    expect((auditLink as HTMLAnchorElement).href).toContain("/audit");
  });

  it("deployment nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewDeployment
        canViewOrganizations
        canViewAllUsers
        canViewHealth
        canViewAuditLog
      />,
    );
    const deploymentMenu = await screen.findByText("Deployment");
    await userEvent.click(deploymentMenu);
    const deploymentSettingsLink = await screen.findByText(
      navLanguage.deployment,
    );
    expect((deploymentSettingsLink as HTMLAnchorElement).href).toContain(
      "/deployment/general",
    );
  });
});
