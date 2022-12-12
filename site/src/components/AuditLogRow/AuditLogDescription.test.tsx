import {
  MockAuditLog,
  MockAuditLogWithWorkspaceBuild,
  MockWorkspaceCreateAuditLogForDifferentOwner,
} from "testHelpers/entities"
import { AuditLogDescription } from "./AuditLogDescription"
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
        workspaceName: "workspace",
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
        `TestUser created workspace bruno-dev on behalf of ${MockWorkspaceCreateAuditLogForDifferentOwner.additional_fields.workspaceOwner}`,
      ),
    ).toBeDefined()
  })
})
