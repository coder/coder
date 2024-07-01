import "chart.js/auto";
import type { Interpolation, Theme } from "@emotion/react";
import type { FC } from "react";
import { Pie } from "react-chartjs-2";
import type { TemplateAppUsage } from "api/typesGenerated";
import { formatTime } from "utils/time";

type AppUsageChartProps = {
  usage: TemplateAppUsage[];
  colors: string[];
};

export const AppUsageChart: FC<AppUsageChartProps> = ({ usage, colors }) => {
  const totalUsageInSeconds = usage.reduce((acc, u) => acc + u.seconds, 0);
  return (
    <Pie
      data={{
        datasets: [
          {
            data: usage.map((u) => u.seconds),
            backgroundColor: colors,
            borderWidth: 1,
          },
        ],
      }}
      options={{
        plugins: {
          tooltip: {
            padding: 12,
            boxPadding: 6,
            usePointStyle: true,
            callbacks: {
              title: (context) => {
                return usage[context[0].dataIndex].display_name;
              },
              label: (context) => {
                const appUsage = usage[context.dataIndex];
                const percentage = Math.round(
                  (appUsage.seconds / totalUsageInSeconds) * 100,
                );
                return `${formatTime(
                  usage[context.dataIndex].seconds,
                )} (${percentage}%)`;
              },
              labelPointStyle: () => {
                return {
                  pointStyle: "circle",
                  rotation: 0,
                };
              },
            },
          },
        },
      }}
    />
  );
};

type AppUsageLabelsProps = {
  usage: TemplateAppUsage[];
  colors: string[];
};

export const AppUsageLabels: FC<AppUsageLabelsProps> = ({ usage, colors }) => {
  return (
    <ul css={styles.list}>
      {usage.map((usage, i) => (
        <li key={usage.slug} css={styles.item}>
          <div css={styles.label}>
            <div css={[styles.labelColor, { backgroundColor: colors[i] }]} />
            <div css={styles.labelIcon}>
              <img src={usage.icon} alt="" />
            </div>
            <span css={styles.labelText}>{usage.display_name}</span>
          </div>
          <div css={styles.info}>
            {formatTime(usage.seconds)}
            {usage.times_used > 0 && (
              <span css={styles.infoSecondary}>
                Opened {usage.times_used.toLocaleString()}{" "}
                {usage.times_used === 1 ? "time" : "times"}
              </span>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
};

const styles = {
  list: {
    flex: 1,
    display: "grid",
    gridAutoRows: "1fr",
    gap: 8,
    margin: 0,
    padding: 0,
  },
  item: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
  },
  label: { display: "flex", alignItems: "center" },
  labelColor: {
    width: 8,
    height: 8,
    borderRadius: 999,
    marginRight: 16,
  },
  labelIcon: {
    width: 20,
    height: 20,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    marginRight: 8,

    "& img": {
      objectFit: "contain",
      width: "100%",
      height: "100%",
    },
  },
  labelText: { fontSize: 13, fontWeight: 500, width: 200 },
  info: (theme) => ({
    fontSize: 13,
    color: theme.palette.text.secondary,
    width: 120,
    flexShrink: 0,
    lineHeight: "1.5",
    display: "flex",
    flexDirection: "column",
  }),
  infoSecondary: (theme) => ({
    fontSize: 12,
    color: theme.palette.text.disabled,
  }),
} satisfies Record<string, Interpolation<Theme>>;
