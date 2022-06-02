import * as TypesGen from "../api/typesGenerated"
import * as Mocks from "../testHelpers/entities"
import { isWorkspaceOn, workspaceQueryToFilter } from "./workspace"

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
  describe("workspaceQueryToFilter", () => {
    it.each<[string | undefined, TypesGen.WorkspaceFilter]>([
      [undefined, {}],
      ["", {}],
      ["asdkfvjn", { name: "asdkfvjn" }],
      ["owner:me", { owner: "me" }],
      ["owner:me owner:me2", { owner: "me" }],
      ["me/dev", { owner: "me", name: "dev" }],
    ])(`query=%p, filter=%p`, (query, filter) => {
      expect(workspaceQueryToFilter(query)).toEqual(filter)
    })
  })
})
