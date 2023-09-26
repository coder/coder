import * as TypesGen from "api/typesGenerated";
import * as Mocks from "testHelpers/entities";
import { displayDormantDeletion } from "./utils";

describe("displayDormantDeletion", () => {
  const today = new Date();
  it.each<[string, boolean, boolean, boolean]>([
    [
      new Date(new Date().setDate(today.getDate() + 15)).toISOString(),
      true,
      true,
      false,
    ], // today + 15 days out
    [
      new Date(new Date().setDate(today.getDate() + 14)).toISOString(),
      true,
      true,
      true,
    ], // today + 14
    [
      new Date(new Date().setDate(today.getDate() + 13)).toISOString(),
      true,
      true,
      true,
    ], // today + 13
    [
      new Date(new Date().setDate(today.getDate() + 1)).toISOString(),
      true,
      true,
      true,
    ], // today + 1
    [new Date().toISOString(), true, true, true], // today + 0
    [new Date().toISOString(), false, true, false], // Advanced Scheduling off
    [new Date().toISOString(), true, false, false], // Workspace Actions off
  ])(
    `deleting_at=%p, allowAdvancedScheduling=%p, AllowWorkspaceActions=%p, shouldDisplay=%p`,
    (
      deleting_at,
      allowAdvancedScheduling,
      allowWorkspaceActions,
      shouldDisplay,
    ) => {
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        deleting_at,
      };
      expect(
        displayDormantDeletion(
          workspace,
          allowAdvancedScheduling,
          allowWorkspaceActions,
        ),
      ).toBe(shouldDisplay);
    },
  );
});
