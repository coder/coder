import { screen } from "@testing-library/react"
import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import { render } from "testHelpers/renderHelpers"
import * as TypesGen from "../../api/typesGenerated"
import * as Mocks from "../../testHelpers/entities"
import {
  canEditDeadline,
  WorkspaceScheduleButton,
} from "./WorkspaceScheduleButton"

dayjs.extend(utc)

describe("WorkspaceScheduleButton", () => {
  describe("shouldDisplayPlusMinus", () => {
    it("should not display if the workspace is not running", () => {
      // Given: a stopped workspace
      const workspace: TypesGen.Workspace = Mocks.MockStoppedWorkspace

      // Then: shouldDisplayPlusMinus should be false
      expect(canEditDeadline(workspace)).toBeFalsy()
    })

    it("should display if the workspace is running", () => {
      // Given: a stopped workspace
      const workspace: TypesGen.Workspace = Mocks.MockWorkspace

      // Then: shouldDisplayPlusMinus should be false
      expect(canEditDeadline(workspace)).toBeTruthy()
    })
  })
  describe("enabling plus and minus buttons", () => {
    it("should enable plus and minus buttons when deadline can be changed in either direction", async () => {
      render(
        <WorkspaceScheduleButton
          workspace={Mocks.MockWorkspace}
          onDeadlineMinus={jest.fn()}
          onDeadlinePlus={jest.fn()}
          maxDeadlineDecrease={4}
          maxDeadlineIncrease={4}
          canUpdateWorkspace
        />,
      )
      const plusButton = await screen.findByLabelText("Add hours to deadline")
      const minusButton = await screen.findByLabelText(
        "Subtract hours from deadline",
      )
      expect(plusButton).toBeEnabled()
      expect(minusButton).toBeEnabled()
    })
    it("should disable plus button when deadline can't be extended", async () => {
      render(
        <WorkspaceScheduleButton
          workspace={Mocks.MockWorkspace}
          onDeadlineMinus={jest.fn()}
          onDeadlinePlus={jest.fn()}
          maxDeadlineDecrease={4}
          maxDeadlineIncrease={0}
          canUpdateWorkspace
        />,
      )
      const plusButton = await screen.findByLabelText("Add hours to deadline")
      const minusButton = await screen.findByLabelText(
        "Subtract hours from deadline",
      )
      expect(plusButton).toBeDisabled()
      expect(minusButton).toBeEnabled()
    })
    it("should disable minus button when deadline can't be reduced", async () => {
      render(
        <WorkspaceScheduleButton
          workspace={Mocks.MockWorkspace}
          onDeadlineMinus={jest.fn()}
          onDeadlinePlus={jest.fn()}
          maxDeadlineDecrease={0}
          maxDeadlineIncrease={4}
          canUpdateWorkspace
        />,
      )
      const plusButton = await screen.findByLabelText("Add hours to deadline")
      const minusButton = await screen.findByLabelText(
        "Subtract hours from deadline",
      )
      expect(plusButton).toBeEnabled()
      expect(minusButton).toBeDisabled()
    })
  })
})
