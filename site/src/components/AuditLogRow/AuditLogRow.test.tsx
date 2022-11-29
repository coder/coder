import { readableActionMessage } from "./AuditLogRow"
import {
  MockAuditLog,
  MockAuditLogWithWorkspaceBuild,
} from "testHelpers/entities"

describe("readableActionMessage()", () => {
  it("renders the correct string for a workspaceBuild audit log", async () => {
    // When
    const friendlyString = readableActionMessage(MockAuditLogWithWorkspaceBuild)

    // Then
    expect(friendlyString).toBe(
      "<strong>TestUser</strong> stopped workspace build for <strong>test2</strong>",
    )
  })
  it("renders the correct string for a workspaceBuild audit log with a duplicate word", async () => {
    // When
    const AuditLogWithRepeat = {
      ...MockAuditLogWithWorkspaceBuild,
      additional_fields: {
        workspaceName: "workspace",
      },
    }
    const friendlyString = readableActionMessage(AuditLogWithRepeat)

    // Then
    expect(friendlyString).toBe(
      "<strong>TestUser</strong> stopped workspace build for <strong>workspace</strong>",
    )
  })
  it("renders the correct string for a workspace audit log", async () => {
    // When
    const friendlyString = readableActionMessage(MockAuditLog)

    // Then
    expect(friendlyString).toBe(
      "<strong>TestUser</strong> created workspace <strong>bruno-dev</strong>",
    )
  })
})
