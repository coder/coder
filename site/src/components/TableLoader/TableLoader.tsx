import { makeStyles } from "@mui/styles";
import TableCell from "@mui/material/TableCell";
import TableRow, { TableRowProps } from "@mui/material/TableRow";
import { FC, ReactNode, cloneElement, isValidElement } from "react";
import { Loader } from "../Loader/Loader";

export const TableLoader: FC = () => {
  const styles = useStyles();

  return (
    <TableRow>
      <TableCell colSpan={999} className={styles.cell}>
        <Loader />
      </TableCell>
    </TableRow>
  );
};

const useStyles = makeStyles((theme) => ({
  cell: {
    textAlign: "center",
    height: theme.spacing(20),
  },
}));

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
