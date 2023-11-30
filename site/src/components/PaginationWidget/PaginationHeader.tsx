import { type FC } from "react";
import { useTheme } from "@emotion/react";
import { type PaginationResult } from "./Pagination";
import Skeleton from "@mui/material/Skeleton";

type PaginationHeaderProps = {
  paginationResult: PaginationResult;
  paginationUnitLabel: string;
};

export const PaginationHeader: FC<PaginationHeaderProps> = ({
  paginationResult,
  paginationUnitLabel,
}) => {
  const theme = useTheme();

  // Need slightly more involved math to account for not having enough data to
  // fill out entire page
  const endBound = Math.min(
    paginationResult.limit - 1,
    (paginationResult.totalRecords ?? 0) - (paginationResult.currentChunk ?? 0),
  );

  return (
    <div
      css={{
        display: "flex",
        flexFlow: "row nowrap",
        alignItems: "center",
        margin: 0,
        fontSize: "13px",
        paddingBottom: "8px",
        color: theme.palette.text.secondary,
        height: "36px", // The size of a small button
        "& strong": {
          color: theme.palette.text.primary,
        },
      }}
    >
      {!paginationResult.isSuccess ? (
        <Skeleton variant="text" width={160} height={16} />
      ) : (
        // This can't be a React fragment because flexbox will rearrange each
        // text node, not the whole thing
        <div>
          Showing {paginationUnitLabel}{" "}
          <strong>
            {paginationResult.currentChunk}&ndash;
            {paginationResult.currentChunk + endBound}
          </strong>{" "}
          (<strong>{paginationResult.totalRecords.toLocaleString()}</strong>{" "}
          {paginationUnitLabel} total)
        </div>
      )}
    </div>
  );
};
