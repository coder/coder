import { screen } from "@testing-library/react"
import {
  MockPrimaryWorkspaceProxy,
  MockUser,
  MockUser2,
} from "../../../testHelpers/entities"
import { renderWithAuth } from "../../../testHelpers/renderHelpers"
import { Language as navLanguage, NavbarView } from "./NavbarView"
import { ProxyContextValue } from "contexts/ProxyContext"
import { action } from "@storybook/addon-actions"

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
}

describe("NavbarView", () => {
  const noop = () => {
    return
  }

  const env = process.env

  // REMARK: copying process.env so we don't mutate that object or encounter conflicts between tests
  beforeEach(() => {
    process.env = { ...env }
  })

  // REMARK: restoring process.env
  afterEach(() => {
    process.env = env
  })

  it("workspaces nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
      />,
    )
    const workspacesLink = await screen.findByText(navLanguage.workspaces)
    expect((workspacesLink as HTMLAnchorElement).href).toContain("/workspaces")
  })

  it("templates nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
      />,
    )
    const templatesLink = await screen.findByText(navLanguage.templates)
    expect((templatesLink as HTMLAnchorElement).href).toContain("/templates")
  })

  it("users nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
      />,
    )
    const userLink = await screen.findByText(navLanguage.users)
    expect((userLink as HTMLAnchorElement).href).toContain("/users")
  })

  it("renders profile picture for user", async () => {
    // Given
    const mockUser = {
      ...MockUser,
      username: "bryan",
      avatar_url: "",
    }

    // When
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={mockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
      />,
    )

    // Then
    // There should be a 'B' avatar!
    const element = await screen.findByText("B")
    expect(element).toBeDefined()
  })

  it("audit nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
      />,
    )
    const auditLink = await screen.findByText(navLanguage.audit)
    expect((auditLink as HTMLAnchorElement).href).toContain("/audit")
  })

  it("audit nav link is hidden for members", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser2}
        onSignOut={noop}
        canViewAuditLog={false}
        canViewDeployment
        canViewAllUsers
      />,
    )
    const auditLink = screen.queryByText(navLanguage.audit)
    expect(auditLink).not.toBeInTheDocument()
  })

  it("deployment nav link has the correct href", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser}
        onSignOut={noop}
        canViewAuditLog
        canViewDeployment
        canViewAllUsers
      />,
    )
    const auditLink = await screen.findByText(navLanguage.deployment)
    expect((auditLink as HTMLAnchorElement).href).toContain(
      "/deployment/general",
    )
  })

  it("deployment nav link is hidden for members", async () => {
    renderWithAuth(
      <NavbarView
        proxyContextValue={proxyContextValue}
        user={MockUser2}
        onSignOut={noop}
        canViewAuditLog={false}
        canViewDeployment={false}
        canViewAllUsers
      />,
    )
    const auditLink = screen.queryByText(navLanguage.deployment)
    expect(auditLink).not.toBeInTheDocument()
  })
})
