import LinearProgress from "@mui/material/LinearProgress";
import Box from "@mui/material/Box";
import { styled, useTheme } from "@mui/material/styles";
import { BoxProps } from "@mui/system";
import { useQuery } from "react-query";
import {
  ActiveUsersTitle,
  ActiveUserChart,
} from "components/ActiveUserChart/ActiveUserChart";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { getLatencyColor } from "utils/latency";
import chroma from "chroma-js";
import { colors } from "theme/colors";
import { Helmet } from "react-helmet-async";
import { getTemplatePageTitle } from "../utils";
import { Loader } from "components/Loader/Loader";
import {
  Template,
  TemplateAppUsage,
  TemplateInsightsResponse,
  TemplateParameterUsage,
  TemplateParameterValue,
  UserActivityInsightsResponse,
  UserLatencyInsightsResponse,
} from "api/typesGenerated";
import { ComponentProps, ReactNode } from "react";
import { subDays, addWeeks } from "date-fns";
import "react-date-range/dist/styles.css";
import "react-date-range/dist/theme/default.css";
import { DateRange as DailyPicker, DateRangeValue } from "./DateRange";
import Link from "@mui/material/Link";
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import CancelOutlined from "@mui/icons-material/CancelOutlined";
import { getDateRangeFilter, lastWeeks } from "./utils";
import Tooltip from "@mui/material/Tooltip";
import LinkOutlined from "@mui/icons-material/LinkOutlined";
import { InsightsInterval, IntervalMenu } from "./IntervalMenu";
import { WeekPicker, numberOfWeeksOptions } from "./WeekPicker";
import {
  insightsTemplate,
  insightsUserActivity,
  insightsUserLatency,
} from "api/queries/insights";
import { useSearchParams } from "react-router-dom";

const DEFAULT_NUMBER_OF_WEEKS = numberOfWeeksOptions[0];

export default function TemplateInsightsPage() {
  const { template } = useTemplateLayoutContext();
  const [searchParams, setSearchParams] = useSearchParams();

  const defaultInterval = getDefaultInterval(template);
  const interval =
    (searchParams.get("interval") as InsightsInterval) || defaultInterval;

  const dateRange = getDateRange(searchParams, interval);
  const setDateRange = (newDateRange: DateRangeValue) => {
    searchParams.set("startDate", newDateRange.startDate.toISOString());
    searchParams.set("endDate", newDateRange.endDate.toISOString());
    setSearchParams(searchParams);
  };

  const commonFilters = {
    template_ids: template.id,
    ...getDateRangeFilter(dateRange),
  };

  const insightsFilter = { ...commonFilters, interval };
  const { data: templateInsights } = useQuery(insightsTemplate(insightsFilter));
  const { data: userLatency } = useQuery(insightsUserLatency(commonFilters));
  const { data: userActivity } = useQuery(insightsUserActivity(commonFilters));

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Insights", template)}</title>
      </Helmet>
      <TemplateInsightsPageView
        controls={
          <>
            <IntervalMenu
              value={interval}
              onChange={(interval) => {
                // When going from daily to week we need to set a safe week range
                if (interval === "week") {
                  setDateRange(lastWeeks(DEFAULT_NUMBER_OF_WEEKS));
                }
                searchParams.set("interval", interval);
                setSearchParams(searchParams);
              }}
            />
            {interval === "day" ? (
              <DailyPicker value={dateRange} onChange={setDateRange} />
            ) : (
              <WeekPicker value={dateRange} onChange={setDateRange} />
            )}
          </>
        }
        templateInsights={templateInsights}
        userLatency={userLatency}
        userActivity={userActivity}
        interval={interval}
      />
    </>
  );
}

const getDefaultInterval = (template: Template) => {
  const now = new Date();
  const templateCreateDate = new Date(template.created_at);
  const hasFiveWeeksOrMore = addWeeks(templateCreateDate, 5) < now;
  return hasFiveWeeksOrMore ? "week" : "day";
};

const getDateRange = (
  searchParams: URLSearchParams,
  interval: InsightsInterval,
) => {
  const startDate = searchParams.get("startDate");
  const endDate = searchParams.get("endDate");

  if (startDate && endDate) {
    return {
      startDate: new Date(startDate),
      endDate: new Date(endDate),
    };
  }

  if (interval === "day") {
    return {
      startDate: subDays(new Date(), 6),
      endDate: new Date(),
    };
  }

  return lastWeeks(DEFAULT_NUMBER_OF_WEEKS);
};

export const TemplateInsightsPageView = ({
  templateInsights,
  userLatency,
  userActivity,
  controls,
  interval,
}: {
  templateInsights: TemplateInsightsResponse | undefined;
  userLatency: UserLatencyInsightsResponse | undefined;
  userActivity: UserActivityInsightsResponse | undefined;
  controls: ReactNode;
  interval: InsightsInterval;
}) => {
  return (
    <>
      <Box
        css={(theme) => ({
          marginBottom: theme.spacing(4),
          display: "flex",
          alignItems: "center",
          gap: theme.spacing(1),
        })}
      >
        {controls}
      </Box>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
          gridTemplateRows: "440px 440px auto",
          gap: (theme) => theme.spacing(3),
        }}
      >
        <ActiveUsersPanel
          sx={{ gridColumn: "span 2" }}
          interval={interval}
          data={templateInsights?.interval_reports}
        />
        <UsersLatencyPanel data={userLatency} />
        <TemplateUsagePanel
          sx={{ gridColumn: "span 2" }}
          data={templateInsights?.report?.apps_usage}
        />
        <UsersActivityPanel data={userActivity} />
        <TemplateParametersUsagePanel
          sx={{ gridColumn: "span 3" }}
          data={templateInsights?.report?.parameters_usage}
        />
      </Box>
    </>
  );
};

const ActiveUsersPanel = ({
  data,
  interval,
  ...panelProps
}: PanelProps & {
  data: TemplateInsightsResponse["interval_reports"] | undefined;
  interval: InsightsInterval;
}) => {
  return (
    <Panel {...panelProps}>
      <PanelHeader>
        <PanelTitle>
          <ActiveUsersTitle />
        </PanelTitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: "100%" }} />}
        {data && data.length === 0 && <NoDataAvailable />}
        {data && data.length > 0 && (
          <ActiveUserChart
            interval={interval}
            data={data.map((d) => ({
              amount: d.active_users,
              date: d.start_time,
            }))}
          />
        )}
      </PanelContent>
    </Panel>
  );
};

const UsersLatencyPanel = ({
  data,
  ...panelProps
}: PanelProps & { data: UserLatencyInsightsResponse | undefined }) => {
  const theme = useTheme();
  const users = data?.report.users;

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
                  alignItems: "center",
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
  );
};

const UsersActivityPanel = ({
  data,
  ...panelProps
}: PanelProps & { data: UserActivityInsightsResponse | undefined }) => {
  const users = data?.report.users;

  return (
    <Panel {...panelProps} sx={{ overflowY: "auto", ...panelProps.sx }}>
      <PanelHeader>
        <PanelTitle sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          Activity by user
          <HelpTooltip size="small">
            <HelpTooltipTitle>How is activity calculated?</HelpTooltipTitle>
            <HelpTooltipText>
              When a connection is initiated to a user&apos;s workspace they are
              considered an active user. e.g. apps, web terminal, SSH
            </HelpTooltipText>
          </HelpTooltip>
        </PanelTitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: "100%" }} />}
        {users && users.length === 0 && <NoDataAvailable />}
        {users &&
          users
            .sort((a, b) => b.seconds - a.seconds)
            .map((row) => (
              <Box
                key={row.user_id}
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
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
                  css={(theme) => ({
                    color: theme.palette.text.secondary,
                    fontSize: 13,
                    textAlign: "right",
                  })}
                >
                  {formatTime(row.seconds)}
                </Box>
              </Box>
            ))}
      </PanelContent>
    </Panel>
  );
};

const TemplateUsagePanel = ({
  data,
  ...panelProps
}: PanelProps & {
  data: TemplateAppUsage[] | undefined;
}) => {
  const validUsage = data?.filter((u) => u.seconds > 0);
  const totalInSeconds =
    validUsage?.reduce((total, usage) => total + usage.seconds, 0) ?? 1;
  const usageColors = chroma
    .scale([colors.green[8], colors.blue[8]])
    .mode("lch")
    .colors(validUsage?.length ?? 0);
  // The API returns a row for each app, even if the user didn't use it.
  const hasDataAvailable = validUsage && validUsage.length > 0;
  return (
    <Panel {...panelProps} css={{ overflowY: "auto" }}>
      <PanelHeader>
        <PanelTitle>App & IDE Usage</PanelTitle>
      </PanelHeader>
      <PanelContent>
        {!data && <Loader sx={{ height: "100%" }} />}
        {data && !hasDataAvailable && <NoDataAvailable />}
        {data && hasDataAvailable && (
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              gap: 3,
            }}
          >
            {validUsage
              .sort((a, b) => b.seconds - a.seconds)
              .map((usage, i) => {
                const percentage = (usage.seconds / totalInSeconds) * 100;
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
                );
              })}
          </Box>
        )}
      </PanelContent>
    </Panel>
  );
};

const TemplateParametersUsagePanel = ({
  data,
  ...panelProps
}: PanelProps & {
  data: TemplateParameterUsage[] | undefined;
}) => {
  return (
    <Panel {...panelProps}>
      <PanelHeader>
        <PanelTitle>Parameters usage</PanelTitle>
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
                : parameter.name;
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
                <Box sx={{ flex: 1 }}>
                  <Box sx={{ fontWeight: 500 }}>{label}</Box>
                  <Box
                    component="p"
                    sx={{
                      fontSize: 14,
                      color: (theme) => theme.palette.text.secondary,
                      maxWidth: 400,
                      margin: 0,
                    }}
                  >
                    {parameter.description}
                  </Box>
                </Box>
                <Box sx={{ flex: 1, fontSize: 14 }}>
                  <ParameterUsageRow
                    sx={{
                      color: (theme) => theme.palette.text.secondary,
                      fontWeight: 500,
                      fontSize: 13,
                      cursor: "default",
                    }}
                  >
                    <Box>Value</Box>
                    <Tooltip
                      title="The number of workspaces using this value"
                      placement="top"
                    >
                      <Box>Count</Box>
                    </Tooltip>
                  </ParameterUsageRow>
                  {parameter.values
                    .sort((a, b) => b.count - a.count)
                    .filter((usage) => filterOrphanValues(usage, parameter))
                    .map((usage, usageIndex) => (
                      <ParameterUsageRow
                        key={`${parameterIndex}-${usageIndex}`}
                      >
                        <ParameterUsageLabel
                          usage={usage}
                          parameter={parameter}
                        />
                        <Box sx={{ textAlign: "right" }}>{usage.count}</Box>
                      </ParameterUsageRow>
                    ))}
                </Box>
              </Box>
            );
          })}
      </PanelContent>
    </Panel>
  );
};

const filterOrphanValues = (
  usage: TemplateParameterValue,
  parameter: TemplateParameterUsage,
) => {
  if (parameter.options) {
    return parameter.options.some((o) => o.value === usage.value);
  }
  return true;
};

const ParameterUsageRow = styled(Box)(({ theme }) => ({
  display: "flex",
  alignItems: "baseline",
  justifyContent: "space-between",
  padding: theme.spacing(0.5, 0),
  gap: theme.spacing(5),
}));

const ParameterUsageLabel = ({
  usage,
  parameter,
}: {
  usage: TemplateParameterValue;
  parameter: TemplateParameterUsage;
}) => {
  if (parameter.options) {
    const option = parameter.options.find((o) => o.value === usage.value)!;
    const icon = option.icon;
    const label = option.name;

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
    );
  }

  if (usage.value.startsWith("http")) {
    return (
      <Link
        href={usage.value}
        target="_blank"
        rel="noreferrer"
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          color: (theme) => theme.palette.text.primary,
        }}
      >
        <TextValue>{usage.value}</TextValue>
        <LinkOutlined
          sx={{
            width: 14,
            height: 14,
            color: (theme) => theme.palette.primary.light,
          }}
        />
      </Link>
    );
  }

  if (parameter.type === "list(string)") {
    const values = JSON.parse(usage.value) as string[];
    return (
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
        {values.map((v, i) => {
          return (
            <Box
              key={i}
              sx={{
                p: (theme) => theme.spacing(0.25, 1.5),
                borderRadius: 999,
                background: (theme) => theme.palette.divider,
                whiteSpace: "nowrap",
              }}
            >
              {v}
            </Box>
          );
        })}
      </Box>
    );
  }

  if (parameter.type === "bool") {
    return (
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
        }}
      >
        {usage.value === "false" ? (
          <>
            <CancelOutlined
              sx={{
                width: 16,
                height: 16,
                color: (theme) => theme.palette.error.light,
              }}
            />
            False
          </>
        ) : (
          <>
            <CheckCircleOutlined
              sx={{
                width: 16,
                height: 16,
                color: (theme) => theme.palette.success.light,
              }}
            />
            True
          </>
        )}
      </Box>
    );
  }

  return <TextValue>{usage.value}</TextValue>;
};

const Panel = styled(Box)(({ theme }) => ({
  borderRadius: theme.shape.borderRadius,
  border: `1px solid ${theme.palette.divider}`,
  backgroundColor: theme.palette.background.paper,
  display: "flex",
  flexDirection: "column",
}));

type PanelProps = ComponentProps<typeof Panel>;

const PanelHeader = styled(Box)(({ theme }) => ({
  padding: theme.spacing(2.5, 3, 3),
}));

const PanelTitle = styled(Box)(() => ({
  fontSize: 14,
  fontWeight: 500,
}));

const PanelContent = styled(Box)(({ theme }) => ({
  padding: theme.spacing(0, 3, 3),
  flex: 1,
}));

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
  );
};

const TextValue = ({ children }: { children: ReactNode }) => {
  return (
    <Box component="span">
      <Box
        component="span"
        sx={{
          color: (theme) => theme.palette.text.secondary,
          weight: 600,
          mr: 0.25,
        }}
      >
        &quot;
      </Box>
      {children}
      <Box
        component="span"
        sx={{
          color: (theme) => theme.palette.text.secondary,
          weight: 600,
          ml: 0.25,
        }}
      >
        &quot;
      </Box>
    </Box>
  );
};

function formatTime(seconds: number): string {
  if (seconds < 60) {
    return seconds + " seconds";
  } else if (seconds >= 60 && seconds < 3600) {
    const minutes = Math.floor(seconds / 60);
    return minutes + " minutes";
  } else {
    const hours = seconds / 3600;
    const minutes = Math.floor(seconds % 3600);
    if (minutes === 0) {
      return hours.toFixed(0) + " hours";
    }

    return hours.toFixed(1) + " hours";
  }
}
