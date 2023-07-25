import LinearProgress from "@mui/material/LinearProgress"
import Box from "@mui/material/Box"
import { styled, useTheme } from "@mui/material/styles"
import { BoxProps } from "@mui/system"
import { useQuery } from "@tanstack/react-query"
import {
  getInsightsTemplate,
  getInsightsUserLatency,
  getTemplateDAUs,
} from "api/api"
import { DAUChart } from "components/DAUChart/DAUChart"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/Tooltips/HelpTooltip"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { getLatencyColor } from "utils/latency"
import chroma from "chroma-js"
import { colors } from "theme/colors"
import { Helmet } from "react-helmet-async"
import { getTemplatePageTitle } from "../utils"

export default function TemplateInsightsPage() {
  const { template } = useTemplateLayoutContext()

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Insights", template)}</title>
      </Helmet>

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
        <TemplateUsagePanel sx={{ gridColumn: "span 3" }} />
      </Box>
    </>
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
                  py: 1,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <UserAvatar
                    username={row.username}
                    avatarURL={row.avatar_url}
                  />
                  <Box sx={{ fontWeight: 500 }}>{row.username}</Box>
                </Box>
                <Box
                  sx={{
                    color: getLatencyColor(theme, row.latency_ms.p95),
                    fontWeight: 500,
                    fontSize: 13,
                    textAlign: "right",
                  }}
                >
                  {row.latency_ms.p95.toFixed(0)}ms
                </Box>
              </Box>
            ))}
      </PanelContent>
    </Panel>
  )
}

const TemplateUsagePanel = (props: BoxProps) => {
  const { template } = useTemplateLayoutContext()
  const { data } = useQuery({
    queryKey: ["templates", template.id, "usage"],
    queryFn: () =>
      getInsightsTemplate({
        template_ids: template.id,
        start_time: toTimeFilter(getTimeFor7DaysAgo()),
        end_time: toTimeFilter(new Date()),
      }),
  })
  const totalInSeconds =
    data?.report.apps_usage.reduce(
      (total, usage) => total + usage.seconds,
      0,
    ) ?? 1
  const usageColors = chroma
    .scale([colors.green[8], colors.blue[8]])
    .mode("lch")
    .colors(data?.report.apps_usage.length ?? 0)
  return (
    <Panel {...props}>
      <PanelHeader>App&lsquo;s & IDE usage</PanelHeader>
      <PanelContent>
        {data && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            {data.report.apps_usage
              .sort((a, b) => b.seconds - a.seconds)
              .map((usage, i) => {
                const percentage = (usage.seconds / totalInSeconds) * 100
                return (
                  <Box
                    key={usage.slug}
                    sx={{ display: "flex", gap: 2, alignItems: "center" }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Box
                        sx={{
                          width: 20,
                          height: 20,
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                        }}
                      >
                        <img
                          src={usage.icon}
                          alt=""
                          style={{
                            objectFit: "contain",
                            width: "100%",
                            height: "100%",
                          }}
                        />
                      </Box>
                      <Box sx={{ fontSize: 13, fontWeight: 500, width: 200 }}>
                        {usage.slug}
                      </Box>
                    </Box>
                    <LinearProgress
                      value={percentage}
                      variant="determinate"
                      sx={{
                        width: "100%",
                        height: 8,
                        backgroundColor: (theme) => theme.palette.divider,
                        "& .MuiLinearProgress-bar": {
                          backgroundColor: usageColors[i],
                          borderRadius: 999,
                        },
                      }}
                    />
                    <Box
                      sx={{
                        fontSize: 13,
                        color: (theme) => theme.palette.text.secondary,
                        width: 200,
                        flexShrink: 0,
                      }}
                    >
                      {formatTime(usage.seconds)}
                    </Box>
                  </Box>
                )
              })}
          </Box>
        )}
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

function formatTime(seconds: number): string {
  if (seconds < 60) {
    return seconds + " seconds"
  } else if (seconds >= 60 && seconds < 3600) {
    const minutes = Math.floor(seconds / 60)
    return minutes + " minutes"
  } else {
    const hours = Math.floor(seconds / 3600)
    const remainingMinutes = Math.floor((seconds % 3600) / 60)
    if (remainingMinutes === 0) {
      return hours + " hours"
    } else {
      return hours + " hours, " + remainingMinutes + " minutes"
    }
  }
}
