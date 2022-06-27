import dayjs from "dayjs"
import * as TypesGen from "../api/typesGenerated"
import * as Mocks from "../testHelpers/entities"
import {
  defaultWorkspaceExtension,
  isWorkspaceDeleted,
  isWorkspaceOn,
  workspaceQueryToFilter,
} from "./workspace"

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
  describe("workspaceQueryToFilter", () => {
    it.each<[string | undefined, TypesGen.WorkspaceFilter]>([
      [undefined, {}],
      ["", { q: "" }],
      ["asdkfvjn", { q: "asdkfvjn" }],
      ["owner:me", { q: "owner:me" }],
      ["owner:me owner:me2", { q: "owner:me owner:me2" }],
      ["me/dev", { q: "me/dev" }],
      ["me/", { q: "me/" }],
      ["    key:val      owner:me       ", { q: "key:val owner:me" }],
    ])(`query=%p, filter=%p`, (query, filter) => {
      expect(workspaceQueryToFilter(query)).toEqual(filter)
    })
  })
})
