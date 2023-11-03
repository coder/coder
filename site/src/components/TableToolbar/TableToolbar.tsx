import { styled } from "@mui/material/styles";
import Box from "@mui/material/Box";
import Skeleton from "@mui/material/Skeleton";

export const TableToolbar = styled(Box)(({ theme }) => ({
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
}));

type BasePaginationStatusProps = {
  label: string;
  isLoading: boolean;
  showing?: number;
  total?: number;
};

type LoadedPaginationStatusProps = BasePaginationStatusProps & {
  isLoading: false;
  showing: number;
  total: number;
};

export const PaginationStatus = ({
  isLoading,
  showing,
  total,
  label,
}: BasePaginationStatusProps | LoadedPaginationStatusProps) => {
  if (isLoading) {
    return (
      <Box sx={{ height: 24, display: "flex", alignItems: "center" }}>
        <Skeleton variant="text" width={160} height={16} />
      </Box>
    );
  }
  return (
    <Box>
      Showing <strong>{showing}</strong> of{" "}
      <strong>{total?.toLocaleString()}</strong> {label}
    </Box>
  );
};
