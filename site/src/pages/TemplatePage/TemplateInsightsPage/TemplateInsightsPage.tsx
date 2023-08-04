import LinearProgress from "@mui/material/LinearProgress"
import Box from "@mui/material/Box"
import { styled, useTheme } from "@mui/material/styles"
import { BoxProps } from "@mui/system"
import { useQuery } from "@tanstack/react-query"
import { getInsightsTemplate, getInsightsUserLatency } from "api/api"
import { DAUChart, DAUTitle } from "components/DAUChart/DAUChart"
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
import { Loader } from "components/Loader/Loader"
import {
  DAUsResponse,
  TemplateInsightsResponse,
  TemplateParameterUsage,
  TemplateParameterValue,
  UserLatencyInsightsResponse,
} from "api/typesGenerated"
import { ComponentProps } from "react"
import subDays from "date-fns/subDays"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined"
import Link from "@mui/material/Link"

export default function TemplateInsightsPage() {
  const { template } = useTemplateLayoutContext()
  const insightsFilter = {
    template_ids: template.id,
    start_time: toTimeFilter(sevenDaysAgo()),
    end_time: toTimeFilter(new Date()),
  }
  const { data: templateInsights } = useQuery({
    queryKey: ["templates", template.id, "usage"],
    queryFn: () => getInsightsTemplate(insightsFilter),
  })
  const { data: userLatency } = useQuery({
    queryKey: ["templates", template.id, "user-latency"],
    queryFn: () => getInsightsUserLatency(insightsFilter),
  })
  const dashboard = useDashboard()
  const shouldDisplayParameters =
    dashboard.experiments.includes("template_parameters_insights") ||
    process.env.NODE_ENV === "development"

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Insights", template)}</title>
      </Helmet>
      <TemplateInsightsPageView
        templateInsights={templateInsights}
        userLatency={userLatency}
        shouldDisplayParameters={shouldDisplayParameters}
      />
    </>
  )
}

export const TemplateInsightsPageView = ({
  templateInsights,
  userLatency,
  shouldDisplayParameters,
}: {
  templateInsights: TemplateInsightsResponse | undefined
  userLatency: UserLatencyInsightsResponse | undefined
  shouldDisplayParameters: boolean
}) => {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
        gridTemplateRows: "440px auto",
        gap: (theme) => theme.spacing(3),
      }}
    >
      <DailyUsersPanel
        sx={{ gridColumn: "span 2" }}
        data={templateInsights?.interval_reports}
      />
      <UserLatencyPanel data={userLatency} />
      <TemplateUsagePanel
        sx={{ gridColumn: "span 3" }}
        data={templateInsights?.report.apps_usage}
      />
      {shouldDisplayParameters && (
        <TemplateParametersUsagePanel
          sx={{ gridColumn: "span 3" }}
          data={templateInsights?.report.parameters_usage}
        />
      )}
    </Box>
  )
}

const DailyUsersPanel = ({
  data,
  ...panelProps
}: PanelProps & {
  data: TemplateInsightsResponse["interval_reports"] | undefined
}) => {
  return (
    <Panel {...panelProps}>
      <PanelHeader>
        <PanelTitle>
          <DAUTitle />
        </PanelTitle>
        <PanelSubtitle>Last 7 days</PanelSubtitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: "100%" }} />}
        {data && data.length === 0 && <NoDataAvailable />}
        {data && data.length > 0 && <DAUChart daus={mapToDAUsResponse(data)} />}
      </PanelContent>
    </Panel>
  )
}

const UserLatencyPanel = ({
  data,
  ...panelProps
}: PanelProps & { data: UserLatencyInsightsResponse | undefined }) => {
  const theme = useTheme()
  const users = data?.report.users

  return (
    <Panel {...panelProps} sx={{ overflowY: "auto", ...panelProps.sx }}>
      <PanelHeader>
        <PanelTitle sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          Latency by user
          <HelpTooltip size="small">
            <HelpTooltipTitle>How is latency calculated?</HelpTooltipTitle>
            <HelpTooltipText>
              The median round trip time of user connections to workspaces.
            </HelpTooltipText>
          </HelpTooltip>
        </PanelTitle>
        <PanelSubtitle>Last 7 days</PanelSubtitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: "100%" }} />}
        {users && users.length === 0 && <NoDataAvailable />}
        {users &&
          users
            .sort((a, b) => b.latency_ms.p50 - a.latency_ms.p50)
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
                    color: getLatencyColor(theme, row.latency_ms.p50),
                    fontWeight: 500,
                    fontSize: 13,
                    textAlign: "right",
                  }}
                >
                  {row.latency_ms.p50.toFixed(0)}ms
                </Box>
              </Box>
            ))}
      </PanelContent>
    </Panel>
  )
}

const TemplateUsagePanel = ({
  data,
  ...panelProps
}: PanelProps & {
  data: TemplateInsightsResponse["report"]["apps_usage"] | undefined
}) => {
  const validUsage = data?.filter((u) => u.seconds > 0)
  const totalInSeconds =
    validUsage?.reduce((total, usage) => total + usage.seconds, 0) ?? 1
  const usageColors = chroma
    .scale([colors.green[8], colors.blue[8]])
    .mode("lch")
    .colors(validUsage?.length ?? 0)
  // The API returns a row for each app, even if the user didn't use it.
  const hasDataAvailable = validUsage && validUsage.length > 0
  return (
    <Panel {...panelProps}>
      <PanelHeader>
        <PanelTitle>App & IDE Usage</PanelTitle>
        <PanelSubtitle>Last 7 days</PanelSubtitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: 200 }} />}
        {data && !hasDataAvailable && <NoDataAvailable sx={{ height: 200 }} />}
        {data && hasDataAvailable && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            {validUsage
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
                        {usage.display_name}
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
                        width: 120,
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

const TemplateParametersUsagePanel = ({
  data,
  ...panelProps
}: PanelProps & {
  data: TemplateInsightsResponse["report"]["parameters_usage"] | undefined
}) => {
  return (
    <Panel {...panelProps}>
      <PanelHeader>
        <PanelTitle>Parameters usage</PanelTitle>
        <PanelSubtitle>Last 7 days</PanelSubtitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: 200 }} />}
        {data && data.length === 0 && <NoDataAvailable sx={{ height: 200 }} />}
        {data &&
          data.length > 0 &&
          data.map((parameter, parameterIndex) => {
            const label =
              parameter.display_name !== ""
                ? parameter.display_name
                : parameter.name
            return (
              <Box
                key={parameter.name}
                sx={{
                  display: "flex",
                  alignItems: "start",
                  p: 3,
                  marginX: -3,
                  borderTop: (theme) => `1px solid ${theme.palette.divider}`,
                  width: (theme) => `calc(100% + ${theme.spacing(6)})`,
                  "&:first-child": {
                    borderTop: 0,
                  },
                }}
              >
                <Box sx={{ fontWeight: 500, flex: 1 }}>{label}</Box>
                <Box sx={{ flex: 1, fontSize: 14 }}>
                  {parameter.values
                    .sort((a, b) => b.count - a.count)
                    .map((value, valueIndex) => (
                      <Box
                        key={`${parameterIndex}-${valueIndex}`}
                        sx={{
                          display: "flex",
                          alignItems: "baseline",
                          justifyContent: "space-between",
                          py: 0.5,
                        }}
                      >
                        <ValueLabel value={value} parameter={parameter} />
                        <Box sx={{ textAlign: "right" }}>{value.count}</Box>
                      </Box>
                    ))}
                </Box>
              </Box>
            )
          })}
      </PanelContent>
    </Panel>
  )
}

const ValueLabel = ({
  value,
  parameter,
}: {
  value: TemplateParameterValue
  parameter: TemplateParameterUsage
}) => {
  if (value.value.trim() === "") {
    return (
      <Box
        component="span"
        sx={{
          color: (theme) => theme.palette.text.secondary,
        }}
      >
        Not set
      </Box>
    )
  }

  if (parameter.options) {
    const option = parameter.options.find((o) => o.value === value.value)!
    const icon = option.icon
    const label = option.name

    return (
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 2,
        }}
      >
        {icon && (
          <Box sx={{ width: 16, height: 16, lineHeight: 1 }}>
            <Box
              component="img"
              src={icon}
              sx={{
                objectFit: "contain",
                width: "100%",
                height: "100%",
              }}
            />
          </Box>
        )}
        {label}
      </Box>
    )
  }

  if (value.value.startsWith("http")) {
    return (
      <Link
        href={value.value}
        target="_blank"
        rel="noreferrer"
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          color: (theme) => theme.palette.text.primary,
        }}
      >
        <OpenInNewOutlined sx={{ width: 14, height: 14 }} />
        {value.value}
      </Link>
    )
  }

  return <Box>{value.value}</Box>
}

const Panel = styled(Box)(({ theme }) => ({
  borderRadius: theme.shape.borderRadius,
  border: `1px solid ${theme.palette.divider}`,
  backgroundColor: theme.palette.background.paper,
  display: "flex",
  flexDirection: "column",
}))

type PanelProps = ComponentProps<typeof Panel>

const PanelHeader = styled(Box)(({ theme }) => ({
  padding: theme.spacing(2.5, 3, 3),
}))

const PanelTitle = styled(Box)(() => ({
  fontSize: 14,
  fontWeight: 500,
}))

const PanelSubtitle = styled(Box)(({ theme }) => ({
  fontSize: 13,
  color: theme.palette.text.secondary,
}))

const PanelContent = styled(Box)(({ theme }) => ({
  padding: theme.spacing(0, 3, 3),
  flex: 1,
}))

const NoDataAvailable = (props: BoxProps) => {
  return (
    <Box
      {...props}
      sx={{
        fontSize: 13,
        color: (theme) => theme.palette.text.secondary,
        textAlign: "center",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        ...props.sx,
      }}
    >
      No data available
    </Box>
  )
}

function mapToDAUsResponse(
  data: TemplateInsightsResponse["interval_reports"],
): DAUsResponse {
  return {
    tz_hour_offset: 0,
    entries: data.map((d) => {
      return {
        amount: d.active_users,
        date: d.end_time,
      }
    }),
  }
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
    const hours = seconds / 3600
    const minutes = Math.floor(seconds % 3600)
    if (minutes === 0) {
      return hours.toFixed(0) + " hours"
    }

    return hours.toFixed(1) + " hours"
  }
}

function sevenDaysAgo() {
  return subDays(new Date(), 7)
}
