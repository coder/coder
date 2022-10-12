import { screen } from "@testing-library/react"
import {
  assignableRole,
  MockAuditorRole,
  MockMemberRole,
  MockOwnerRole,
  MockTemplateAdminRole,
  MockUserAdminRole,
  render,
} from "testHelpers/renderHelpers"
import { RoleSelect } from "./RoleSelect"

describe("UserRoleSelect", () => {
  it("renders content", async () => {
    // When
    render(
      <RoleSelect
        roles={[
          assignableRole(MockOwnerRole, false),
          assignableRole(MockTemplateAdminRole, false),
          assignableRole(MockAuditorRole, true),
          assignableRole(MockUserAdminRole, true),
        ]}
        selectedRoles={[
          MockUserAdminRole,
          MockTemplateAdminRole,
          MockMemberRole,
        ]}
        loading={false}
        onChange={jest.fn()}
        open
      />,
    )

    // Then
    const owner = await screen.findByText(MockOwnerRole.display_name)
    const templateAdmin = await screen.findByText(
      MockTemplateAdminRole.display_name,
    )
    const auditor = await screen.findByText(MockAuditorRole.display_name)
    const userAdmin = await screen.findByText(MockUserAdminRole.display_name)

    // The attributes are "strings", not boolean types.
    expect(owner.getAttribute("aria-disabled")).toBe("true")
    expect(templateAdmin.getAttribute("aria-disabled")).toBe("true")

    expect(userAdmin.getAttribute("aria-disabled")).toBe("false")
    expect(auditor.getAttribute("aria-disabled")).toBe("false")
  })
})
