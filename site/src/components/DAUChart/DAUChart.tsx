import Box from "@mui/material/Box";
import { Theme } from "@mui/material/styles";
import useTheme from "@mui/styles/useTheme";
import * as TypesGen from "api/typesGenerated";
import {
  CategoryScale,
  Chart as ChartJS,
  ChartOptions,
  defaults,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  TimeScale,
  Title,
  Tooltip,
  PointElement,
} from "chart.js";
import "chartjs-adapter-date-fns";
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import dayjs from "dayjs";
import { FC } from "react";
import { Line } from "react-chartjs-2";

ChartJS.register(
  CategoryScale,
  LinearScale,
  TimeScale,
  LineElement,
  PointElement,
  Filler,
  Title,
  Tooltip,
  Legend,
);

export interface DAUChartProps {
  daus: TypesGen.DAUsResponse;
}

export const DAUChart: FC<DAUChartProps> = ({ daus }) => {
  const theme: Theme = useTheme();

  const labels = daus.entries.map((val) => {
    return dayjs(val.date).format("YYYY-MM-DD");
  });

  const data = daus.entries.map((val) => {
    return val.amount;
  });

  defaults.font.family = theme.typography.fontFamily as string;
  defaults.color = theme.palette.text.secondary;

  const options: ChartOptions<"line"> = {
    responsive: true,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        displayColors: false,
        callbacks: {
          title: (context) => {
            const date = new Date(context[0].parsed.x);
            return date.toLocaleDateString();
          },
        },
      },
    },
    scales: {
      y: {
        suggestedMin: 0,
        ticks: {
          precision: 0,
        },
      },

      x: {
        ticks: {
          stepSize: daus.entries.length > 10 ? 2 : undefined,
        },
        type: "time",
        time: {
          unit: "day",
        },
      },
    },
    maintainAspectRatio: false,
  };

  return (
    <Line
      data-chromatic="ignore"
      data={{
        labels: labels,
        datasets: [
          {
            label: "Daily Active Users",
            data: data,
            pointBackgroundColor: theme.palette.info.light,
            pointBorderColor: theme.palette.info.light,
            borderColor: theme.palette.info.light,
            backgroundColor: theme.palette.info.dark,
            fill: "origin",
          },
        ],
      }}
      options={options}
    />
  );
};

export const DAUTitle = () => {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
      Daily Active Users
      <HelpTooltip size="small">
        <HelpTooltipTitle>
          How do we calculate daily active users?
        </HelpTooltipTitle>
        <HelpTooltipText>
          When a connection is initiated to a user&apos;s workspace they are
          considered a daily active user. e.g. apps, web terminal, SSH
        </HelpTooltipText>
      </HelpTooltip>
    </Box>
  );
};
