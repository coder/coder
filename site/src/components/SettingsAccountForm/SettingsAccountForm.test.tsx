import { screen } from "@testing-library/react"
import { MockUser2 } from "../../testHelpers/entities"
import { render } from "../../testHelpers/renderHelpers"
import { AccountForm, AccountFormValues } from "./SettingsAccountForm"

// NOTE: it does not matter what the role props of MockUser are set to,
//       only that editable is set to true or false. This is passed from
//       the call to /authorization done by authXService
describe("AccountForm", () => {
  describe("when editable is set to true", () => {
    it("allows updating username", async () => {
      // Given
      const mockInitialValues: AccountFormValues = {
        username: MockUser2.username,
        editable: true,
      }

      // When
      render(
        <AccountForm
          email={MockUser2.email}
          initialValues={mockInitialValues}
          isLoading={false}
          onSubmit={() => {
            return
          }}
        />,
      )

      // Then
      const el = await screen.findByLabelText("Username")
      expect(el).toBeEnabled()
    })
  })

  describe("when editable is set to false", () => {
    it("does not allow updating username", async () => {
      // Given
      const mockInitialValues: AccountFormValues = {
        username: MockUser2.username,
        editable: false,
      }

      // When
      render(
        <AccountForm
          email={MockUser2.email}
          initialValues={mockInitialValues}
          isLoading={false}
          onSubmit={() => {
            return
          }}
        />,
      )

      // Then
      const el = await screen.findByLabelText("Username")
      expect(el).toBeDisabled()
    })
  })
})
