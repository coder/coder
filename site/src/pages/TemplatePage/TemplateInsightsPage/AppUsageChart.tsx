import "chart.js/auto";
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
            borderWidth: 0,
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
