import * as TypesGen from "api/typesGenerated";

export const queryToFilter = (
  query?: string,
): TypesGen.WorkspaceFilter | TypesGen.UsersRequest => {
  return {
    q: prepareQuery(query),
  };
};

export const prepareQuery = (query?: string) => {
  return query?.trim().replace(/  +/g, " ");
};

export const workspaceFilterQuery = {
  me: "owner:me",
  all: "",
  running: "status:running",
  failed: "status:failed",
  dormant: "is-dormant:true",
};

export const userFilterQuery = {
  active: "status:active",
  all: "",
};
