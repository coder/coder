import dayjs from "dayjs";
import * as TypesGen from "api/typesGenerated";
import * as Mocks from "testHelpers/entities";
import {
  agentVersionStatus,
  defaultWorkspaceExtension,
  getDisplayVersionStatus,
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceTemplateName,
  isWorkspaceOn,
} from "./workspace";

describe("util > workspace", () => {
  describe("isWorkspaceOn", () => {
    it.each<
      [TypesGen.WorkspaceTransition, TypesGen.ProvisionerJobStatus, boolean]
    >([
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
    ])(
      `transition=%p, status=%p, isWorkspaceOn=%p`,
      (transition, status, isOn) => {
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
        };
        expect(isWorkspaceOn(workspace)).toBe(isOn);
      },
    );
  });

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
      expect(defaultWorkspaceExtension(dayjs(startTime))).toEqual(request);
    });
  });

  describe("getDisplayWorkspaceBuildInitiatedBy", () => {
    it.each<[TypesGen.WorkspaceBuild, string]>([
      [Mocks.MockWorkspaceBuild, "TestUser"],
      [
        {
          ...Mocks.MockWorkspaceBuild,
          reason: "autostart",
        },
        "Coder",
      ],
      [
        {
          ...Mocks.MockWorkspaceBuild,
          reason: "autostop",
        },
        "Coder",
      ],
    ])(
      `getDisplayWorkspaceBuildInitiatedBy(%p) returns %p`,
      (build, initiatedBy) => {
        expect(getDisplayWorkspaceBuildInitiatedBy(build)).toEqual(initiatedBy);
      },
    );
  });

  describe("getDisplayVersionStatus", () => {
    it.each<[string, string, string, string, string, agentVersionStatus]>([
      ["", "", "", "", "Unknown", agentVersionStatus.Updated],
      ["", "v1.2.3", "", "", "Unknown", agentVersionStatus.Updated],
      ["v1.2.3", "", "", "", "v1.2.3", agentVersionStatus.Updated],
      ["v1.2.3", "v1.2.3", "", "", "v1.2.3", agentVersionStatus.Updated],
      ["v1.2.3", "v1.2.4", "", "", "v1.2.3", agentVersionStatus.Outdated],
      ["v1.2.4", "v1.2.3", "", "", "v1.2.4", agentVersionStatus.Updated],
      ["foo", "bar", "", "", "foo", agentVersionStatus.Updated],
      [
        "v1.2.3",
        "v1.2.4",
        "1.8",
        "2.1",
        "v1.2.3",
        agentVersionStatus.Deprecated,
      ],
    ])(
      `getDisplayVersionStatus(theme, %p, %p, %p, %p) returns (%p, %p)`,
      (
        agentVersion,
        serverVersion,
        agentAPIVersion,
        serverAPIVersion,
        expectedVersion,
        expectedStatus,
      ) => {
        const { displayVersion, status } = getDisplayVersionStatus(
          agentVersion,
          serverVersion,
          agentAPIVersion,
          serverAPIVersion,
        );
        expect(displayVersion).toEqual(expectedVersion);
        expect(status).toEqual(expectedStatus);
      },
    );
  });

  describe("getDisplayWorkspaceTemplateName", () => {
    it("display name is not set", async () => {
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        template_display_name: "",
      };
      const displayed = getDisplayWorkspaceTemplateName(workspace);
      expect(displayed).toEqual(workspace.template_name);
    });
    it("display name is set", async () => {
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
      };
      const displayed = getDisplayWorkspaceTemplateName(workspace);
      expect(displayed).toEqual(workspace.template_display_name);
    });
  });
});
