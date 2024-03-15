import type * as TypesGen from "api/typesGenerated";
import * as Mocks from "testHelpers/entities";
import { displayDormantDeletion } from "./dormant";

describe("displayDormantDeletion", () => {
  const today = new Date();
  it.each<[string, boolean, boolean]>([
    [
      new Date(new Date().setDate(today.getDate() + 15)).toISOString(),
      true,
      false,
    ], // today + 15 days out
    [
      new Date(new Date().setDate(today.getDate() + 14)).toISOString(),
      true,
      true,
    ], // today + 14
    [
      new Date(new Date().setDate(today.getDate() + 13)).toISOString(),
      true,
      true,
    ], // today + 13
    [
      new Date(new Date().setDate(today.getDate() + 1)).toISOString(),
      true,
      true,
    ], // today + 1
    [new Date().toISOString(), true, true], // today + 0
    [new Date().toISOString(), false, false], // Advanced Scheduling off
  ])(
    `deleting_at=%p, allowAdvancedScheduling=%p, shouldDisplay=%p`,
    (deleting_at, allowAdvancedScheduling, shouldDisplay) => {
      const workspace: TypesGen.Workspace = {
        ...Mocks.MockWorkspace,
        deleting_at,
      };
      expect(displayDormantDeletion(workspace, allowAdvancedScheduling)).toBe(
        shouldDisplay,
      );
    },
  );
});
