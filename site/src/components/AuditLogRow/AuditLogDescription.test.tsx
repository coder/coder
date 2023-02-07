import {
  MockAuditLog,
  MockAuditLogWithWorkspaceBuild,
  MockWorkspaceCreateAuditLogForDifferentOwner,
  MockAuditLogSuccessfulLogin,
  MockAuditLogUnsuccessfulLoginKnownUser,
  MockAuditLogUnsuccessfulLoginUnknownUser,
} from "testHelpers/entities"
import { AuditLogDescription } from "./AuditLogDescription"
import { AuditLogRow } from "./AuditLogRow"
import { render } from "../../testHelpers/renderHelpers"
import { screen } from "@testing-library/react"

const getByTextContent = (text: string) => {
  return screen.getByText((_, element) => {
    const hasText = (element: Element | null) => element?.textContent === text
    const elementHasText = hasText(element)
    const childrenDontHaveText = Array.from(element?.children || []).every(
      (child) => !hasText(child),
    )
    return elementHasText && childrenDontHaveText
  })
}
describe("AuditLogDescription", () => {
  it("renders the correct string for a workspace create audit log", async () => {
    render(<AuditLogDescription auditLog={MockAuditLog} />)

    expect(
      getByTextContent("TestUser created workspace bruno-dev"),
    ).toBeDefined()
  })

  it("renders the correct string for a workspace_build stop audit log", async () => {
    render(<AuditLogDescription auditLog={MockAuditLogWithWorkspaceBuild} />)

    expect(
      getByTextContent("TestUser stopped build for workspace test2"),
    ).toBeDefined()
  })

  it("renders the correct string for a workspace_build audit log with a duplicate word", async () => {
    const AuditLogWithRepeat = {
      ...MockAuditLogWithWorkspaceBuild,
      additional_fields: {
        workspace_name: "workspace",
      },
    }
    render(<AuditLogDescription auditLog={AuditLogWithRepeat} />)

    expect(
      getByTextContent("TestUser stopped build for workspace workspace"),
    ).toBeDefined()
  })
  it("renders the correct string for a workspace created for a different owner", async () => {
    render(
      <AuditLogDescription
        auditLog={MockWorkspaceCreateAuditLogForDifferentOwner}
      />,
    )
    expect(
      getByTextContent(
        `TestUser created workspace bruno-dev on behalf of ${MockWorkspaceCreateAuditLogForDifferentOwner.additional_fields.workspace_owner}`,
      ),
    ).toBeDefined()
  })
  it("renders the correct string for successful login", async () => {
    render(<AuditLogRow auditLog={MockAuditLogSuccessfulLogin} />)
    expect(getByTextContent(`TestUser logged in`)).toBeDefined()
    const statusPill = screen.getByRole("status")
    expect(statusPill).toHaveTextContent("201")
  })
  it("renders the correct string for unsuccessful login for a known user", async () => {
    render(<AuditLogRow auditLog={MockAuditLogUnsuccessfulLoginKnownUser} />)
    expect(getByTextContent(`TestUser logged in`)).toBeDefined()
    const statusPill = screen.getByRole("status")
    expect(statusPill).toHaveTextContent("401")
  })
  it("renders the correct string for unsuccessful login for an unknown user", async () => {
    render(<AuditLogRow auditLog={MockAuditLogUnsuccessfulLoginUnknownUser} />)
    expect(getByTextContent(`an unknown user logged in`)).toBeDefined()
    const statusPill = screen.getByRole("status")
    expect(statusPill).toHaveTextContent("401")
  })
})
