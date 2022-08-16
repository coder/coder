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
        selectedRoles={[MockUserAdminRole, MockTemplateAdminRole, MockMemberRole]}
        loading={false}
        onChange={jest.fn()}
        open={true}
      />,
    )

    // Then
    const owner = await screen.findByText(MockOwnerRole.display_name)
    const templateAdmin = await screen.findByText(MockTemplateAdminRole.display_name)
    const auditor = await screen.findByText(MockAuditorRole.display_name)
    const userAdmin = await screen.findByText(MockUserAdminRole.display_name)

    expect(owner).toHaveProperty("disabled", true)
    expect(templateAdmin).toHaveProperty("disabled", true)
    expect(auditor).toHaveProperty("disabled", true)
    expect(userAdmin).toHaveProperty("disabled", true)
  })
})
