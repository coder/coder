import { fireEvent, screen } from "@testing-library/react"
import { Language } from "components/Tooltips/OutdatedHelpTooltip"
import { WorkspaceStats } from "components/WorkspaceStats/WorkspaceStats"
import { MockOutdatedWorkspace } from "testHelpers/entities"
import { renderWithAuth } from "testHelpers/renderHelpers"
import * as CreateDayString from "util/createDayString"

describe("WorkspaceStats", () => {
  it("shows an outdated tooltip", async () => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")

    const handleUpdateMock = jest.fn()
    renderWithAuth(
      <WorkspaceStats handleUpdate={handleUpdateMock} workspace={MockOutdatedWorkspace} />,
      {
        route: `/@${MockOutdatedWorkspace.owner_name}/${MockOutdatedWorkspace.name}`,
        path: "/@:username/:workspace",
      },
    )
    const tooltipButton = await screen.findByRole("button")
    fireEvent.click(tooltipButton)
    expect(await screen.findByText(Language.versionTooltipText)).toBeInTheDocument()
    const updateButton = screen.getByRole("button", {
      name: "update version",
    })
    fireEvent.click(updateButton)
    expect(handleUpdateMock).toBeCalledTimes(1)
  })
})
