import { styled } from "@mui/material/styles";
import Box from "@mui/material/Box";
import Skeleton from "@mui/material/Skeleton";

export const TableToolbar = styled(Box)(({ theme }) => ({
  fontSize: 13,
  marginBottom: theme.spacing(1),
  marginTop: theme.spacing(0),
  height: 36, // The size of a small button
  color: theme.palette.text.secondary,
  "& strong": { color: theme.palette.text.primary },
  display: "flex",
  alignItems: "center",
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
