import { WorkspaceSection } from "components/WorkspaceSection/WorkspaceSection"
import { FC } from "react"

import moment from "moment"
import { Line } from "react-chartjs-2"

import * as TypesGen from "../../api/typesGenerated"

export interface DAUChartProps {
  userMetricsData: TypesGen.GetDAUsResponse
}

import useTheme from "@material-ui/styles/useTheme"

import { Theme } from "@material-ui/core/styles"
import {
  CategoryScale,
  Chart as ChartJS,
  ChartOptions,
  defaults,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Title,
  Tooltip,
} from "chart.js"
import { Stack } from "components/Stack/Stack"
import { HelpTooltip, HelpTooltipText, HelpTooltipTitle } from "components/Tooltips/HelpTooltip"

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend)

export const DAUChart: FC<DAUChartProps> = ({ userMetricsData }) => {
  const theme: Theme = useTheme()

  if (userMetricsData.entries.length === 0) {
    return (
      <div style={{ marginTop: "-20px" }}>
        <p>DAU stats are loading. Check back later.</p>
      </div>
    )
  }

  const labels = userMetricsData.entries.map((val) => {
    return moment(val.date).format("l")
  })

  const data = userMetricsData.entries.map((val) => {
    return val.daus
  })

  defaults.font.family = theme.typography.fontFamily

  const options = {
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
      },
    },
    aspectRatio: 6 / 1,
  } as ChartOptions

  return (
    <>
      {/* <WorkspaceSection title="Daily Active Users"> */}
      <WorkspaceSection>
        <Stack direction="row" spacing={1} alignItems="center">
          <h3>Daily Active Users</h3>
          <HelpTooltip size="small">
            <HelpTooltipTitle>How do we calculate DAUs?</HelpTooltipTitle>
            <HelpTooltipText>
              We use daily, unique workspace connection traffic to compute DAUs.
            </HelpTooltipText>
          </HelpTooltip>
        </Stack>
        <Line
          data={{
            labels: labels,
            datasets: [
              {
                label: "Daily Active Users",
                data: data,
                lineTension: 1 / 4,
                backgroundColor: theme.palette.secondary.dark,
                borderColor: theme.palette.secondary.dark,
              },
            ] as any,
          }}
          options={options as any}
          height={400}
        />
      </WorkspaceSection>
    </>
  )
}
