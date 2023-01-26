import { useSearchParams } from "react-router-dom"

type UseFilterResult = {
  query: string
  setFilter: (query: string) => void
}

export const useFilter = (defaultValue: string): UseFilterResult => {
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get("filter") ?? defaultValue

  const setFilter = (query: string) => {
    searchParams.set("filter", query)
    setSearchParams(searchParams)
  }

  return {
    query,
    setFilter,
  }
}
