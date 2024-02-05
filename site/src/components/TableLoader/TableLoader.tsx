import TableCell from "@mui/material/TableCell";
import TableRow, { type TableRowProps } from "@mui/material/TableRow";
import { type FC, type ReactNode, cloneElement, isValidElement } from "react";
import { Loader } from "../Loader/Loader";

export const TableLoader: FC = () => {
  return (
    <TableRow>
      <TableCell colSpan={999} css={{ textAlign: "center", height: 160 }}>
        <Loader />
      </TableCell>
    </TableRow>
  );
};

interface TableLoaderSkeletonProps {
  rows?: number;
  children?: ReactNode;
}

export const TableLoaderSkeleton: FC<TableLoaderSkeletonProps> = ({
  rows = 4,
  children,
}) => {
  if (!isValidElement(children)) {
    throw new Error(
      "TableLoaderSkeleton children must be a valid React element",
    );
  }
  return (
    <>
      {Array.from({ length: rows }, (_, i) =>
        cloneElement(children, { key: i }),
      )}
    </>
  );
};

export const TableRowSkeleton: FC<TableRowProps> = ({
  children,
  ...rowProps
}) => {
  return (
    <TableRow role="progressbar" data-testid="loader" {...rowProps}>
      {children}
    </TableRow>
  );
};
