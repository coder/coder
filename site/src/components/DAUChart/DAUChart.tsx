import Box from "@mui/material/Box"
import { Theme } from "@mui/material/styles"
import useTheme from "@mui/styles/useTheme"
import * as TypesGen from "api/typesGenerated"
import {
  CategoryScale,
  Chart as ChartJS,
  ChartOptions,
  defaults,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  TimeScale,
  Title,
  Tooltip,
} from "chart.js"
import "chartjs-adapter-date-fns"
import {
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipText,
} from "components/Tooltips/HelpTooltip"
import dayjs from "dayjs"
import { FC } from "react"
import { Line } from "react-chartjs-2"

ChartJS.register(
  CategoryScale,
  LinearScale,
  TimeScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
)

export interface DAUChartProps {
  daus: TypesGen.DAUsResponse
}

export const DAUChart: FC<DAUChartProps> = ({ daus }) => {
  const theme: Theme = useTheme()

  const labels = daus.entries.map((val) => {
    return dayjs(val.date).format("YYYY-MM-DD")
  })

  const data = daus.entries.map((val) => {
    return val.amount
  })

  defaults.font.family = theme.typography.fontFamily as string
  defaults.color = theme.palette.text.secondary

  const options: ChartOptions<"line"> = {
    responsive: true,
    plugins: {
      legend: {
        display: false,
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
        ticks: {},
        type: "time",
        time: {
          unit: "day",
          stepSize: 2,
        },
      },
    },
    maintainAspectRatio: false,
  }

  return (
    <Line
      data-chromatic="ignore"
      data={{
        labels: labels,
        datasets: [
          {
            label: "Daily Active Users",
            data: data,
            tension: 1 / 4,
            backgroundColor: theme.palette.secondary.dark,
            borderColor: theme.palette.secondary.dark,
          },
        ],
      }}
      options={options}
    />
  )
}

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
  )
}
