import Box from "@mui/material/Box"
import Skeleton from "@mui/material/Skeleton"

type BasePaginationStatusProps = {
  label: string
  isLoading: boolean
  showing?: number
  total?: number
}

type LoadedPaginationStatusProps = BasePaginationStatusProps & {
  isLoading: false
  showing: number
  total: number
}

export const PaginationStatus = ({
  isLoading,
  showing,
  total,
  label,
}: BasePaginationStatusProps | LoadedPaginationStatusProps) => {
  return (
    <Box
      sx={{
        fontSize: 13,
        mb: 2,
        mt: 1,
        color: (theme) => theme.palette.text.secondary,
        "& strong": { color: (theme) => theme.palette.text.primary },
      }}
    >
      {!isLoading ? (
        <>
          Showing <strong>{showing}</strong> of{" "}
          <strong>{total?.toLocaleString()}</strong> {label}
        </>
      ) : (
        <Box sx={{ height: 24, display: "flex", alignItems: "center" }}>
          <Skeleton variant="text" width={160} height={16} />
        </Box>
      )}
    </Box>
  )
}
