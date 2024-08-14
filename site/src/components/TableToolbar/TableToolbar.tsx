import Skeleton from "@mui/material/Skeleton";
import type { FC, PropsWithChildren } from "react";

export const TableToolbar: FC<PropsWithChildren> = ({ children }) => {
  return (
    <div
      css={(theme) => ({
        fontSize: 13,
        marginBottom: "8px",
        marginTop: 0,
        height: "36px", // The size of a small button
        color: theme.palette.text.secondary,
        display: "flex",
        alignItems: "center",
        "& strong": {
          color: theme.palette.text.primary,
        },
      })}
    >
      {children}
    </div>
  );
};

type PaginationStatusProps =
  | BasePaginationStatusProps
  | LoadedPaginationStatusProps;

type BasePaginationStatusProps = {
  isLoading: true;
};

type LoadedPaginationStatusProps = {
  isLoading: false;
  label: string;
  showing: number;
  total: number;
};

export const PaginationStatus: FC<PaginationStatusProps> = (props) => {
  const { isLoading } = props;

  if (isLoading) {
    return (
      <div css={{ height: 24, display: "flex", alignItems: "center" }}>
        <Skeleton variant="text" width={160} height={16} />
      </div>
    );
  }

  const { showing, total, label } = props;

  return (
    <div>
      Showing <strong>{showing}</strong> of{" "}
      <strong>{total?.toLocaleString()}</strong> {label}
    </div>
  );
};
