import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as TypesGen from "../../api/typesGenerated"
import * as Mocks from "../../testHelpers/entities"
import {
  deadlineMinusDisabled,
  deadlinePlusDisabled,
  shouldDisplayPlusMinus,
} from "./WorkspaceSchedule"

dayjs.extend(utc)
const now = dayjs()

describe("WorkspaceSchedule", () => {
  describe("shouldDisplayPlusMinus", () => {
    it("should not display if the workspace is not running", () => {
      // Given: a stopped workspace
      const workspace: TypesGen.Workspace = Mocks.MockStoppedWorkspace

      // Then: shouldDisplayPlusMinus should be false
      expect(shouldDisplayPlusMinus(workspace)).toBeFalsy()
    })

    it("should display if the workspace is running", () => {
      // Given: a stopped workspace
      const workspace: TypesGen.Workspace = Mocks.MockWorkspace

      // Then: shouldDisplayPlusMinus should be false
      expect(shouldDisplayPlusMinus(workspace)).toBeTruthy()
    })
  })

  describe("deadlineMinusDisabled", () => {
    it("should be false if the deadline is more than 30 minutes in the future", () => {
      // Given: a workspace with a deadline set to 31 minutes in the future
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: now.add(31, "minutes").utc().format(),
        },
      }

      // Then: deadlineMinusDisabled should be falsy
      expect(deadlineMinusDisabled(workspace, now)).toBeFalsy()
    })

    it("should be true if the deadline is 30 minutes or less in the future", () => {
      // Given: a workspace with a deadline set to 30 minutes in the future
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: now.add(30, "minutes").utc().format(),
        },
      }

      // Then: deadlineMinusDisabled should be truthy
      expect(deadlineMinusDisabled(workspace, now)).toBeTruthy()
    })
  })

  describe("deadlinePlusDisabled", () => {
    it("should be false if the deadline is less than 24 hours in the future", () => {
      // Given: a workspace with a deadline set to 23 hours in the future
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: now.add(23, "hours").utc().format(),
        },
      }

      // Then: deadlinePlusDisabled should be falsy
      expect(deadlinePlusDisabled(workspace, now)).toBeFalsy()
    })

    it("should be true if the deadline is 24 hours or more in the future", () => {
      // Given: a workspace with a deadline set to 25 hours in the future
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: now.add(25, "hours").utc().format(),
        },
      }

      // Then: deadlinePlusDisabled should be truthy
      expect(deadlinePlusDisabled(workspace, now)).toBeTruthy()
    })
  })
})
