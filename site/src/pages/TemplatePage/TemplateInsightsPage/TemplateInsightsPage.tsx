import Box from "@mui/material/Box"
import { styled } from "@mui/material/styles"

const TemplateInsightsPage = () => {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
        gridTemplateRows: "440px auto",
        gap: (theme) => theme.spacing(3),
      }}
    >
      <Panel sx={{ gridColumn: "span 2" }}>
        <PanelHeader>Active daily users</PanelHeader>
      </Panel>

      <Panel>
        <PanelHeader>Latency by user</PanelHeader>
      </Panel>

      <Panel sx={{ gridColumn: "span 3" }}>
        <PanelHeader>App&lsquo;s & IDE usage</PanelHeader>
      </Panel>
    </Box>
  )
}

export default TemplateInsightsPage

const Panel = styled(Box)(({ theme }) => ({
  borderRadius: theme.shape.borderRadius,
  border: `1px solid ${theme.palette.divider}`,
  backgroundColor: theme.palette.background.paper,
}))

const PanelHeader = styled(Box)(({ theme }) => ({
  fontSize: 14,
  fontWeight: 500,
  padding: theme.spacing(3),
  lineHeight: 1,
}))
