import { useTheme } from "@mui/material/styles"
import HelpOutline from "@mui/icons-material/HelpOutline"
import Box from "@mui/material/Box"
import Tooltip from "@mui/material/Tooltip"
import { FC } from "react"
import { getLatencyColor } from "utils/latency"

export const ProxyStatusLatency: FC<{ latency?: number }> = ({ latency }) => {
  const theme = useTheme()
  const color = getLatencyColor(theme, latency)

  if (!latency) {
    return (
      <Tooltip title="Latency not available">
        <HelpOutline
          sx={{
            ml: "auto",
            fontSize: "14px !important",
            color,
          }}
        />
      </Tooltip>
    )
  }

  return (
    <Box sx={{ color, fontSize: 13, marginLeft: "auto" }}>
      {latency.toFixed(0)}ms
    </Box>
  )
}
