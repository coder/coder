import type { useSearchParams } from "react-router-dom";
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils";

export const usePagination = ({
  searchParamsResult,
}: {
  searchParamsResult: ReturnType<typeof useSearchParams>;
}) => {
  const [searchParams, setSearchParams] = searchParamsResult;
  const page = searchParams.get("page") ? Number(searchParams.get("page")) : 1;
  const limit = DEFAULT_RECORDS_PER_PAGE;
  const offset = page <= 0 ? 0 : (page - 1) * limit;

  const goToPage = (page: number) => {
    searchParams.set("page", page.toString());
    setSearchParams(searchParams);
  };

  return {
    page,
    limit,
    goToPage,
    offset,
  };
};
