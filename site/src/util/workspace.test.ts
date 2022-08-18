import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import * as TypesGen from "../api/typesGenerated"
import * as Mocks from "../testHelpers/entities"
import {
  defaultWorkspaceExtension,
  getDisplayWorkspaceBuildInitiatedBy,
  isWorkspaceDeleted,
  isWorkspaceOn,
  maxDeadline,
  minDeadline,
  deadlineExtensionMax,
  deadlineExtensionMin,
} from "./workspace"

dayjs.extend(duration)
const now = dayjs()

describe("util > workspace", () => {
  describe("isWorkspaceOn", () => {
    it.each<[TypesGen.WorkspaceTransition, TypesGen.ProvisionerJobStatus, boolean]>([
      ["delete", "canceled", false],
      ["delete", "canceling", false],
      ["delete", "failed", false],
      ["delete", "pending", false],
      ["delete", "running", false],
      ["delete", "succeeded", false],

      ["stop", "canceled", false],
      ["stop", "canceling", false],
      ["stop", "failed", false],
      ["stop", "pending", false],
      ["stop", "running", false],
      ["stop", "succeeded", false],

      ["start", "canceled", false],
      ["start", "canceling", false],
      ["start", "failed", false],
      ["start", "pending", false],
      ["start", "running", false],
      ["start", "succeeded", true],
    ])(`transition=%p, status=%p, isWorkspaceOn=%p`, (transition, status, isOn) => {
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          job: {
            ...Mocks.MockProvisionerJob,
            status,
          },
          transition,
        },
      }
      expect(isWorkspaceOn(workspace)).toBe(isOn)
    })
  })

  describe("isWorkspaceDeleted", () => {
    it.each<[TypesGen.WorkspaceTransition, TypesGen.ProvisionerJobStatus, boolean]>([
      ["delete", "canceled", false],
      ["delete", "canceling", false],
      ["delete", "failed", false],
      ["delete", "pending", false],
      ["delete", "running", false],
      ["delete", "succeeded", true],

      ["stop", "canceled", false],
      ["stop", "canceling", false],
      ["stop", "failed", false],
      ["stop", "pending", false],
      ["stop", "running", false],
      ["stop", "succeeded", false],

      ["start", "canceled", false],
      ["start", "canceling", false],
      ["start", "failed", false],
      ["start", "pending", false],
      ["start", "running", false],
      ["start", "succeeded", false],
    ])(`transition=%p, status=%p, isWorkspaceDeleted=%p`, (transition, status, isDeleted) => {
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        latest_build: {
          ...Mocks.MockWorkspaceBuild,
          job: {
            ...Mocks.MockProvisionerJob,
            status,
          },
          transition,
        },
      }
      expect(isWorkspaceDeleted(workspace)).toBe(isDeleted)
    })
  })

  describe("defaultWorkspaceExtension", () => {
    it.each<[string, TypesGen.PutExtendWorkspaceRequest]>([
      [
        "2022-06-02T14:56:34Z",
        {
          deadline: "2022-06-02T18:56:34Z",
        },
      ],

      // This case is the same as above, but in a different timezone to prove
      // that UTC conversion for deadline works as expected
      [
        "2022-06-02T10:56:20-04:00",
        {
          deadline: "2022-06-02T18:56:20Z",
        },
      ],
    ])(`defaultWorkspaceExtension(%p) returns %p`, (startTime, request) => {
      expect(defaultWorkspaceExtension(dayjs(startTime))).toEqual(request)
    })
  })

  describe("getDisplayWorkspaceBuildInitiatedBy", () => {
    it.each<[TypesGen.WorkspaceBuild, string]>([
      [Mocks.MockWorkspaceBuild, "TestUser"],
      [
        {
          ...Mocks.MockWorkspaceBuild,
          reason: "autostart",
        },
        "system/autostart",
      ],
      [
        {
          ...Mocks.MockWorkspaceBuild,
          reason: "autostop",
        },
        "system/autostop",
      ],
    ])(`getDisplayWorkspaceBuildInitiatedBy(%p) returns %p`, (build, initiatedBy) => {
      expect(getDisplayWorkspaceBuildInitiatedBy(build)).toEqual(initiatedBy)
    })
  })
})

describe("maxDeadline", () => {
  // Given: a workspace built from a template with a max deadline equal to 25 hours which isn't really possible
  const workspace: TypesGen.Workspace = {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      deadline: now.add(8, "hours").utc().format(),
    },
  }
  describe("given a template with 25 hour max ttl", () => {
    it("should be never be greater than global max deadline", () => {
      const template: TypesGen.Template = {
        ...Mocks.MockTemplate,
        max_ttl_ms: 25 * 60 * 60 * 1000,
      }

      // Then: deadlineMinusDisabled should be falsy
      const delta = maxDeadline(workspace, template).diff(now)
      expect(delta).toBeLessThanOrEqual(deadlineExtensionMax.asMilliseconds())
    })
  })

  describe("given a template with 4 hour max ttl", () => {
    it("should be never be greater than global max deadline", () => {
      const template: TypesGen.Template = {
        ...Mocks.MockTemplate,
        max_ttl_ms: 4 * 60 * 60 * 1000,
      }

      // Then: deadlineMinusDisabled should be falsy
      const delta = maxDeadline(workspace, template).diff(now)
      expect(delta).toBeLessThanOrEqual(deadlineExtensionMax.asMilliseconds())
    })
  })
})

describe("minDeadline", () => {
  it("should never be less than 30 minutes", () => {
    const delta = minDeadline().diff(now)
    expect(delta).toBeGreaterThanOrEqual(deadlineExtensionMin.asMilliseconds())
  })
})
