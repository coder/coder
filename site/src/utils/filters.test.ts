import * as TypesGen from "api/typesGenerated";
import { queryToFilter } from "./filters";

describe("queryToFilter", () => {
  it.each<
    [string | undefined, TypesGen.WorkspaceFilter | TypesGen.UsersRequest]
  >([
    [undefined, {}],
    ["", { q: "" }],
    ["asdkfvjn", { q: "asdkfvjn" }],
    ["owner:me", { q: "owner:me" }],
    ["owner:me owner:me2", { q: "owner:me owner:me2" }],
    ["me/dev", { q: "me/dev" }],
    ["me/", { q: "me/" }],
    ["    key:val      owner:me       ", { q: "key:val owner:me" }],
    ["status:failed", { q: "status:failed" }],
  ])(`query=%p, filter=%p`, (query, filter) => {
    expect(queryToFilter(query)).toEqual(filter);
  });
});
