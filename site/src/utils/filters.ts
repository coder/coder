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
  dormant: "dormant_at:1970-01-01",
};

export const userFilterQuery = {
  active: "status:active",
  all: "",
};
