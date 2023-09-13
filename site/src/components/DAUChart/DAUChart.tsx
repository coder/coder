import Box from "@mui/material/Box";
import { Theme } from "@mui/material/styles";
import useTheme from "@mui/styles/useTheme";
import * as TypesGen from "api/typesGenerated";
import {
  CategoryScale,
  Chart as ChartJS,
  ChartOptions,
  defaults,
  Legend,
  LinearScale,
  BarElement,
  TimeScale,
  Title,
  Tooltip,
} from "chart.js";
import "chartjs-adapter-date-fns";
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import dayjs from "dayjs";
import { FC } from "react";
import { Bar } from "react-chartjs-2";

ChartJS.register(
  CategoryScale,
  LinearScale,
  TimeScale,
  BarElement,
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

  const options: ChartOptions<"bar"> = {
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
        min: 0,
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
    <Bar
      data-chromatic="ignore"
      data={{
        labels: labels,
        datasets: [
          {
            label: "Daily Active Users",
            data: data,
            backgroundColor: theme.palette.secondary.dark,
            borderColor: theme.palette.secondary.dark,
            barThickness: 8,
            borderWidth: 2,
            borderRadius: Number.MAX_VALUE,
            borderSkipped: false,
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
          When a connection is initiated to a user{"'"}s workspace they are
          considered a daily active user. e.g. apps, web terminal, SSH
        </HelpTooltipText>
      </HelpTooltip>
    </Box>
  );
};
