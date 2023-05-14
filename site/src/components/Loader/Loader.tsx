import Box from "@mui/material/Box"
import CircularProgress from "@mui/material/CircularProgress"
import { FC } from "react"

export const Loader: FC<React.PropsWithChildren<{ size?: number }>> = ({
  size = 26,
}) => {
  return (
    <Box
      p={4}
      width="100%"
      display="flex"
      alignItems="center"
      justifyContent="center"
      data-testid="loader"
    >
      <CircularProgress size={size} />
    </Box>
  )
}
