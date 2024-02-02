import { screen } from "@testing-library/react";
import { MockPrimaryWorkspaceProxy, MockUser } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { Language as navLanguage, NavbarView } from "./NavbarView";
import { ProxyContextValue } from "contexts/ProxyContext";
import { action } from "@storybook/addon-actions";

const proxyContextValue: ProxyContextValue = {
  proxy: {
    preferredPathAppURL: "",
    preferredWildcardHostname: "",
    proxy: MockPrimaryWorkspaceProxy,
  },
  isLoading: false,
  isFetched: true,
  setProxy: jest.fn(),
  clearProxy: action("clearProxy"),
  refetchProxyLatencies: jest.fn(),
  proxyLatencies: {},
};

describe("NavbarView", () => {
  const noop = () => {
    return;
  };

  it("workspaces nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
        canViewHealth
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
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
        canViewHealth
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
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
        canViewHealth
      />,
    );
    const userLink = await screen.findByText(navLanguage.users);
    expect((userLink as HTMLAnchorElement).href).toContain("/users");
  });

  it("audit nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
        canViewHealth
      />,
    );
    const auditLink = await screen.findByText(navLanguage.audit);
    expect((auditLink as HTMLAnchorElement).href).toContain("/audit");
  });

  it("deployment nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
        canViewHealth
      />,
    );
    const auditLink = await screen.findByText(navLanguage.deployment);
    expect((auditLink as HTMLAnchorElement).href).toContain(
      "/deployment/general",
    );
  });
});
