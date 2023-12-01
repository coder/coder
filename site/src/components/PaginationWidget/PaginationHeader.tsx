import { type FC } from "react";
import { useTheme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";

type PaginationHeaderProps = {
  paginationUnitLabel: string;
  limit: number;
  totalRecords: number | undefined;
  currentOffsetStart: number | undefined;
};

export const PaginationHeader: FC<PaginationHeaderProps> = ({
  paginationUnitLabel,
  limit,
  totalRecords,
  currentOffsetStart,
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
      {totalRecords !== undefined ? (
        <>
          {/**
           * Have to put text content in divs so that flexbox doesn't scramble
           * the inner text nodes up
           */}
          {totalRecords === 0 && <div>No records available</div>}

          {totalRecords !== 0 && currentOffsetStart !== undefined && (
            <div>
              Showing {paginationUnitLabel}{" "}
              <strong>
                {currentOffsetStart}&ndash;
                {currentOffsetStart +
                  Math.min(limit - 1, totalRecords - currentOffsetStart)}
              </strong>{" "}
              (<strong>{totalRecords.toLocaleString()}</strong>{" "}
              {paginationUnitLabel} total)
            </div>
          )}
        </>
      ) : (
        <Skeleton variant="text" width={160} height={16} />
      )}
    </div>
  );
};
