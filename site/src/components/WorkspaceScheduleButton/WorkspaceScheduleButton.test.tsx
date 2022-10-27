import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as TypesGen from "../../api/typesGenerated"
import * as Mocks from "../../testHelpers/entities"
import { canEditDeadline } from "./WorkspaceScheduleButton"

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
})
