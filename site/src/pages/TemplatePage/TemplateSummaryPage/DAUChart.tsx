import { Theme } from "@material-ui/core/styles"
import useTheme from "@material-ui/styles/useTheme"
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
import { Stack } from "components/Stack/Stack"
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip"
import { WorkspaceSection } from "components/WorkspaceSection/WorkspaceSection"
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
  templateDAUs: TypesGen.TemplateDAUsResponse
}
export const Language = {
  loadingText: "DAU stats are loading. Check back later.",
  chartTitle: "Daily Active Users",
}

export const DAUChart: FC<DAUChartProps> = ({
  templateDAUs: templateMetricsData,
}) => {
  const theme: Theme = useTheme()

  if (templateMetricsData.entries.length === 0) {
    return (
      // We generate hidden element to prove this path is taken in the test
      // and through site inspection.
      <div style={{ display: "none" }}>
        <p>{Language.loadingText}</p>
      </div>
    )
  }

  const labels = templateMetricsData.entries.map((val) => {
    return dayjs(val.date).format("YYYY-MM-DD")
  })

  const data = templateMetricsData.entries.map((val) => {
    return val.amount
  })

  defaults.font.family = theme.typography.fontFamily as string
  defaults.color = theme.palette.text.secondary

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
        type: "time",
        time: {
          unit: "day",
          stepSize: 2,
        },
      },
    },
    aspectRatio: 10 / 1,
  } as ChartOptions

  return (
    <>
      <WorkspaceSection
        title={
          <Stack direction="row" spacing={1} alignItems="center">
            {Language.chartTitle}
            <HelpTooltip size="small">
              <HelpTooltipTitle>How do we calculate DAUs?</HelpTooltipTitle>
              <HelpTooltipText>
                We use all workspace connection traffic to calculate DAUs.
              </HelpTooltipText>
            </HelpTooltip>
          </Stack>
        }
      >
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
              // There are type bugs in chart.js that force us to use any.
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
            ] as any,
          }}
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          options={options as any}
          height={400}
        />
      </WorkspaceSection>
    </>
  )
}
