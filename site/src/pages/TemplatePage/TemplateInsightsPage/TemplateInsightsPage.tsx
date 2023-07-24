import Box from "@mui/material/Box"
import { styled, useTheme } from "@mui/material/styles"
import { BoxProps } from "@mui/system"
import { useQuery } from "@tanstack/react-query"
import { getInsightsUserLatency, getTemplateDAUs } from "api/api"
import { DAUChart } from "components/DAUChart/DAUChart"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/Tooltips/HelpTooltip"
import { getLatencyColor } from "utils/latency"

export default function TemplateInsightsPage() {
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
      <UserLatencyPanel />

      <Panel sx={{ gridColumn: "span 3" }}>
        <PanelHeader>App&lsquo;s & IDE usage</PanelHeader>
      </Panel>
    </Box>
  )
}

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

const UserLatencyPanel = (props: BoxProps) => {
  const { template } = useTemplateLayoutContext()
  const { data } = useQuery({
    queryKey: ["templates", template.id, "user-latency"],
    queryFn: () =>
      getInsightsUserLatency({
        template_ids: template.id,
        start_time: toTimeFilter(getTimeFor7DaysAgo()),
        end_time: toTimeFilter(new Date()),
      }),
  })
  const theme = useTheme()

  return (
    <Panel {...props} sx={{ overflowY: "auto", ...props.sx }}>
      <PanelHeader sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        Latency by user
      </PanelHeader>
      <PanelContent>
        {data &&
          data.report.users
            .sort((a, b) => b.latency_ms.p95 - a.latency_ms.p95)
            .map((row) => (
              <Box
                key={row.user_id}
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  fontSize: 14,
                }}
              >
                <Box sx={{ fontWeight: 500 }}>{row.username}</Box>
                <Box sx={{ color: getLatencyColor(theme, row.latency_ms.p95) }}>
                  {row.latency_ms.p95.toFixed(0)}ms
                </Box>
              </Box>
            ))}
      </PanelContent>
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

function getTimeFor7DaysAgo(): Date {
  const sevenDaysAgo = new Date()
  sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
  return sevenDaysAgo
}

function toTimeFilter(date: Date) {
  date.setHours(0, 0, 0, 0)
  const year = date.getUTCFullYear()
  const month = String(date.getUTCMonth() + 1).padStart(2, "0")
  const day = String(date.getUTCDate()).padStart(2, "0")

  return `${year}-${month}-${day}T00:00:00Z`
}
