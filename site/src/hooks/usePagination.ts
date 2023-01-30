import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils"
import { useSearchParams } from "react-router-dom"

type UsePaginationResult = {
  page: number
  limit: number
  goToPage: (page: number) => void
}

export const usePagination = (): UsePaginationResult => {
  const [searchParams, setSearchParams] = useSearchParams()
  const page = searchParams.get("page") ? Number(searchParams.get("page")) : 0
  const limit = DEFAULT_RECORDS_PER_PAGE

  const goToPage = (page: number) => {
    searchParams.set("page", page.toString())
    setSearchParams(searchParams)
  }

  return {
    page,
    limit,
    goToPage,
  }
}
