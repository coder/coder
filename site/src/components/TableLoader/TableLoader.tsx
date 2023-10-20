import TableCell from "@mui/material/TableCell";
import TableRow, { type TableRowProps } from "@mui/material/TableRow";
import { type FC, type ReactNode, cloneElement, isValidElement } from "react";
import { useTheme } from "@emotion/react";
import { Loader } from "../Loader/Loader";

export const TableLoader: FC = () => {
  const theme = useTheme();

  return (
    <TableRow>
      <TableCell
        colSpan={999}
        css={{
          textAlign: "center",
          height: theme.spacing(20),
        }}
      >
        <Loader />
      </TableCell>
    </TableRow>
  );
};

export const TableLoaderSkeleton = ({
  rows = 4,
  children,
}: {
  rows?: number;
  children: ReactNode;
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

export const TableRowSkeleton = (props: TableRowProps) => {
  return <TableRow role="progressbar" data-testid="loader" {...props} />;
};
