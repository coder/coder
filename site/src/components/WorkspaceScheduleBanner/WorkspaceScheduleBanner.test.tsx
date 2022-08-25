import dayjs from "dayjs"
import utc from "dayjs/plugin/utc"
import * as TypesGen from "../../api/typesGenerated"
import * as Mocks from "../../testHelpers/entities"
import { shouldDisplay } from "./WorkspaceScheduleBanner"

dayjs.extend(utc)

describe("WorkspaceScheduleBanner", () => {
  describe("shouldDisplay", () => {
    // Manual TTL case
    it("should not display if the build does not have a deadline", () => {
      // Given: a workspace with deadline of undefined.
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: undefined,
          transition: "start",
        },
      }

      // Then: shouldDisplay is false
      expect(shouldDisplay(workspace)).toBeFalsy()
    })

    // Transition Checks
    it("should not display if the latest build is not transition=start", () => {
      // Given: a workspace with latest build as "stop"
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          transition: "stop",
        },
      }

      // Then: shouldDisplay is false
      expect(shouldDisplay(workspace)).toBeFalsy()
    })

    // Provisioner Job Checks
    it("should not display if the latest build is canceling", () => {
      // Given: a workspace with latest build as "canceling"
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          job: Mocks.MockCancelingProvisionerJob,
          transition: "start",
        },
      }

      // Then: shouldDisplay is false
      expect(shouldDisplay(workspace)).toBeFalsy()
    })
    it("should not display if the latest build is canceled", () => {
      // Given: a workspace with latest build as "canceled"
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          job: Mocks.MockCanceledProvisionerJob,
          transition: "start",
        },
      }

      // Then: shouldDisplay is false
      expect(shouldDisplay(workspace)).toBeFalsy()
    })
    it("should not display if the latest build failed", () => {
      // Given: a workspace with latest build as "failed"
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          job: Mocks.MockFailedProvisionerJob,
          transition: "start",
        },
      }

      // Then: shouldDisplay is false
      expect(shouldDisplay(workspace)).toBeFalsy()
    })

    // Deadline Checks
    it("should display if deadline is within 30 minutes", () => {
      // Given: a workspace with latest build as start and deadline in ~30 mins
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: dayjs().add(27, "minutes").utc().format(),
          transition: "start",
        },
      }

      // Then: shouldDisplay is true
      expect(shouldDisplay(workspace)).toBeTruthy()
    })
    it("should not display if deadline is 45 minutes", () => {
      // Given: a workspace with latest build as start and deadline in 45 mins
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          deadline: dayjs().add(45, "minutes").utc().format(),
          transition: "start",
        },
      }

      // Then: shouldDisplay is false
      expect(shouldDisplay(workspace)).toBeFalsy()
    })
  })
})
