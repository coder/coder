import { type FC } from "react";
import { useTheme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";

type PaginationHeaderProps = {
  paginationUnitLabel: string;
  limit: number;
  totalRecords: number | undefined;
  currentChunk: number | undefined;
};

export const PaginationHeader: FC<PaginationHeaderProps> = ({
  paginationUnitLabel,
  limit,
  totalRecords,
  currentChunk,
}) => {
  const theme = useTheme();

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
      {currentChunk !== undefined && totalRecords !== undefined ? (
        // This can't be a React fragment because flexbox will rearrange each
        // text node, instead of all the elements as a group
        <div>
          Showing {paginationUnitLabel}{" "}
          <strong>
            {currentChunk}&ndash;
            {currentChunk + Math.min(limit - 1, totalRecords - currentChunk)}
          </strong>{" "}
          (<strong>{totalRecords.toLocaleString()}</strong>{" "}
          {paginationUnitLabel} total)
        </div>
      ) : (
        <Skeleton variant="text" width={160} height={16} />
      )}
    </div>
  );
};
