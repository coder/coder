/**
 * @file Mock input props for use with PaginationContainer's tests and stories.
 *
 * Had to split this off into a separate file because housing these in the test
 * file and then importing them from the stories file was causing Chromatic's
 * Vite test environment to break
 */
import type { PaginationResult } from "./PaginationContainer";

type ResultBase = Omit<
  PaginationResult,
  "isPreviousData" | "currentOffsetStart" | "totalRecords" | "totalPages"
>;

export const mockPaginationResultBase: ResultBase = {
  isSuccess: false,
  currentPage: 1,
  limit: 25,
  hasNextPage: false,
  hasPreviousPage: false,
  goToPreviousPage: () => {},
  goToNextPage: () => {},
  goToFirstPage: () => {},
  onPageChange: () => {},
};

export const mockInitialRenderResult: PaginationResult = {
  ...mockPaginationResultBase,
  isSuccess: false,
  isPreviousData: false,
  currentOffsetStart: undefined,
  hasNextPage: false,
  hasPreviousPage: false,
  totalRecords: undefined,
  totalPages: undefined,
};

export const mockSuccessResult: PaginationResult = {
  ...mockPaginationResultBase,
  isSuccess: true,
  isPreviousData: false,
  currentOffsetStart: 1,
  totalPages: 1,
  totalRecords: 4,
};
