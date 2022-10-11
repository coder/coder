import { useSearchParams } from "react-router-dom"
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
}

export const userFilterQuery = {
  active: "status:active",
  all: "",
}

export const useFilter = (
  defaultFilter: string,
): {
  filter: string
  setFilter: (filter: string) => void
} => {
  const [searchParams, setSearchParams] = useSearchParams()
  const filter = searchParams.get("filter") ?? defaultFilter

  const setFilter = (filter: string) => {
    setSearchParams({ filter })
  }

  return {
    filter,
    setFilter,
  }
}
