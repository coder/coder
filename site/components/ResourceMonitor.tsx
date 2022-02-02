import React from "react"
import { Bar, Line } from "react-chartjs-2"
import { Chart, ChartOptions } from "chart.js"

const multiply = {
  beforeDraw: function (chart: Chart, options: ChartOptions) {
    if (chart && chart.ctx) {
      chart.ctx.globalCompositeOperation = "multiply"
    }
  },
  afterDatasetsDraw: function (chart: Chart, options: ChartOptions) {
    if (chart && chart.ctx) {
      chart.ctx.globalCompositeOperation = "source-over"
    }
  },
}

function formatBytes(bytes: number, decimals = 2) {
  if (bytes === 0) return "0 Bytes"

  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ["Bytes", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"]

  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + " " + sizes[i]
}

const padding = 64

const opts: ChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  legend: {
    fullWidth: true,
    display: false,
  },
  elements: {
    point: {
      radius: 0,
      hitRadius: 8,
      hoverRadius: 8,
    },
    rectangle: {
      borderWidth: 0,
    },
  },
  layout: {
    padding: {
      top: padding,
      bottom: padding,
    },
  },
  tooltips: {
    mode: "index",
    axis: "y",
    cornerRadius: 8,
    borderWidth: 0,
    titleFontStyle: "normal",
    callbacks: {
      label: (item: any, data: any) => {
        const dataset = data.datasets[item.datasetIndex]
        const num: number = dataset.data[item.index] as number
        if (num) {
          return dataset.label + ": " + num.toFixed(2) + "%"
        }
      },
      labelColor: (item: any, data: any) => {
        const dataset = data.data.datasets[item.datasetIndex]
        return {
          // Trim off the transparent hex code.
          backgroundColor: (dataset.pointBackgroundColor as string).substr(0, 7),
          borderColor: "#000000",
        }
      },
      title: (item) => {
        console.log(item[0])

        return "Resources: " + item[0].label
      },
    },
  },
  plugins: {
    tooltip: {
      callbacks: {
        beforeTitle: (item: any) => {
          console.log("BEFORE TITLE: " + item)
          return "Resources"
        },
      },
    },
    legend: {
      display: false,
    },
  },
  scales: {
    xAxes: [
      {
        display: false,
        ticks: {
          stepSize: 10,
          maxTicksLimit: 4,
          maxRotation: 0,
        },
      },
    ],
    yAxes: [
      {
        gridLines: {
          color: "rgba(0, 0, 0, 0.09)",
          zeroLineColor: "rgba(0, 0, 0, 0.09)",
        },
        ticks: {
          callback: (v) => v + "%",
          max: 100,
          maxTicksLimit: 2,
          min: 0,
          padding: 4,
        },
      },
    ],
  },
}

export interface ResourceUsageSnapshot {
  cpuPercentage: number
  memoryUsedBytes: number
  diskUsedBytes: number
}

export interface ResourceMonitorProps {
  readonly diskTotalBytes: number
  readonly memoryTotalBytes: number
  readonly resources: ReadonlyArray<ResourceUsageSnapshot>
}

export const ResourceMonitor: React.FC<ResourceMonitorProps> = (props) => {
  const dataF = React.useMemo(() => {
    return (canvas: any) => {
      // Store gradients inside the canvas object for easy access.
      // This function is called everytime resources values change...
      // we don't want to allocate a new gradient everytime.
      if (!canvas["cpuGradient"]) {
        const cpuGradient = canvas.getContext("2d").createLinearGradient(0, 0, 0, canvas.height)
        cpuGradient.addColorStop(1, "#9787FF32")
        cpuGradient.addColorStop(0, "#5555FFC4")
        canvas["cpuGradient"] = cpuGradient
      }

      if (!canvas["memGradient"]) {
        const memGradient = canvas.getContext("2d").createLinearGradient(0, 0, 0, canvas.height)
        memGradient.addColorStop(1, "#55FF8532")
        memGradient.addColorStop(0, "#42B863C4")
        canvas["memGradient"] = memGradient
      }

      if (!canvas["diskGradient"]) {
        const diskGradient = canvas.getContext("2d").createLinearGradient(0, 0, 0, canvas.height)
        diskGradient.addColorStop(1, "#97979700")
        diskGradient.addColorStop(0, "#797979C4")
        canvas["diskGradient"] = diskGradient
      }

      const cpuPercentages = []
      //const cpuPercentages = Array(20 - props.resources.length).fill(null)
      cpuPercentages.push(...props.resources.map((r) => r.cpuPercentage))

      //const memPercentages = Array(20 - props.resources.length).fill(null)
      const memPercentages = []
      memPercentages.push(...props.resources.map((r) => (r.memoryUsedBytes / props.memoryTotalBytes) * 100))

      const diskPercentages = []
      //const diskPercentages = Array(20 - props.resources.length).fill(null)
      diskPercentages.push(...props.resources.map((r) => (r.diskUsedBytes / props.diskTotalBytes) * 100))

      return {
        labels: Array(20)
          .fill(0)
          .map((_, index) => (20 - index) * 3 + "s ago"),
        datasets: [
          {
            label: "CPU",
            data: cpuPercentages,
            backgroundColor: canvas["cpuGradient"],
            borderColor: "transparent",
            pointBackgroundColor: "#9787FF32",
            pointBorderColor: "#FFFFFF",
            lineTension: 0.4,
            fill: true,
          },
          {
            label: "Memory",
            data: memPercentages,
            backgroundColor: canvas["memGradient"],
            borderColor: "transparent",
            pointBackgroundColor: "#55FF8532",
            pointBorderColor: "#FFFFFF",
            lineTension: 0.4,
            fill: true,
          },
          {
            label: "Disk",
            data: diskPercentages,
            backgroundColor: canvas["diskGradient"],
            borderColor: "transparent",
            pointBackgroundColor: "#97979732",
            pointBorderColor: "#FFFFFF",
            lineTension: 0.4,
            fill: true,
          },
        ],
      }
    }
  }, [props.resources])

  return (
    <Line
      type="line"
      height={40 + padding * 2}
      data={dataF}
      options={opts}
      plugins={[multiply]}
      ref={(ref) => {
        window.Chart.defaults.global.defaultFontFamily = "'Fira Code', Inter"
      }}
    />
  )
}
