import * as TypesGen from "../api/typesGenerated"

export const queryToFilter = (
  query?: string,
): TypesGen.WorkspaceFilter | TypesGen.UsersRequest => {
  const preparedQuery = query?.trim().replace(/  +/g, " ")
  return {
    q: preparedQuery,
  }
}

export const workspaceFilterQuery = {
  me: "owner:me",
  all: "",
  running: "status:running",
}

export const userFilterQuery = {
  active: "status:active",
  all: "",
}
