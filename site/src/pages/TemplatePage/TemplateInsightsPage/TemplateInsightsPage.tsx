import Box from "@mui/material/Box"
import { styled } from "@mui/material/styles"
import { BoxProps } from "@mui/system"
import { useQuery } from "@tanstack/react-query"
import { getTemplateDAUs } from "api/api"
import { DAUChart } from "components/DAUChart/DAUChart"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/Tooltips/HelpTooltip"

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
      <DailyUsersPanel sx={{ gridColumn: "span 2" }} />

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

const DailyUsersPanel = (props: BoxProps) => {
  const { template } = useTemplateLayoutContext()
  const { data } = useQuery({
    queryKey: ["templates", template.id, "dau"],
    queryFn: () => getTemplateDAUs(template.id),
  })
  return (
    <Panel {...props}>
      <PanelHeader sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        Active daily users
        <HelpTooltip size="small">
          <HelpTooltipTitle>How do we calculate DAUs?</HelpTooltipTitle>
          <HelpTooltipText>
            We use all workspace connection traffic to calculate DAUs.
          </HelpTooltipText>
        </HelpTooltip>
      </PanelHeader>
      <PanelContent>{data && <DAUChart daus={data} />}</PanelContent>
    </Panel>
  )
}

const Panel = styled(Box)(({ theme }) => ({
  borderRadius: theme.shape.borderRadius,
  border: `1px solid ${theme.palette.divider}`,
  backgroundColor: theme.palette.background.paper,
  display: "flex",
  flexDirection: "column",
}))

const PanelHeader = styled(Box)(({ theme }) => ({
  fontSize: 14,
  fontWeight: 500,
  padding: theme.spacing(3),
  lineHeight: 1,
}))

const PanelContent = styled(Box)(({ theme }) => ({
  padding: theme.spacing(0, 3, 3),
  flex: 1,
}))
