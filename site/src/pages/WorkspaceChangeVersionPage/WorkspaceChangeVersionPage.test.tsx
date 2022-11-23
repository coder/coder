import {
  MockTemplateVersion2,
  MockUser,
  MockWorkspace,
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import WorkspaceChangeVersionPage from "./WorkspaceChangeVersionPage"
import { screen, waitFor } from "@testing-library/react"
import * as API from "api/api"
import userEvent from "@testing-library/user-event"
import * as CreateDayString from "util/createDayString"
import i18next from "i18next"

const t = (path: string) => {
  return i18next.t(path, { ns: "workspaceChangeVersionPage" })
}

const renderPage = async () => {
  renderWithAuth(<WorkspaceChangeVersionPage />, {
    path: "/@:username/:workspace/change-version",
    route: `/@${MockUser.username}/${MockWorkspace.name}/change-version`,
  })
  await waitForLoaderToBeRemoved()
}

describe("WorkspaceChangeVersionPage", () => {
  beforeEach(() => {
    jest
      .spyOn(CreateDayString, "createDayString")
      .mockImplementation(() => "a minute ago")
  })

  it("sends the update request with the right version", async () => {
    const user = userEvent.setup()
    const updateSpy = jest.spyOn(API, "startWorkspace")
    await renderPage()

    // Type the version name and select it
    const autocompleteInput = screen.getByLabelText(
      t("labels.workspaceVersion"),
    )
    await user.clear(autocompleteInput)
    await user.type(autocompleteInput, MockTemplateVersion2.name)
    const newOption = screen.getByRole("option", {
      // Using RegExp so we can match a substring
      name: new RegExp(MockTemplateVersion2.name),
    })
    await user.click(newOption)

    // Submit the form
    const submitButton = screen.getByRole("button", {
      name: t("labels.submit"),
    })
    await user.click(submitButton)

    await waitFor(() => {
      expect(updateSpy).toBeCalledWith(
        MockWorkspace.id,
        MockTemplateVersion2.id,
      )
    })
  })
})
