import { Region } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"

export const usePreferredProxy = (): Region | undefined => {
  const dashboard = useDashboard()
  // Only use preferred proxy if the user has the moons experiment enabled
  if(!dashboard?.experiments.includes("moons")) {
    return undefined
  }

  const str = localStorage.getItem("preferred-proxy")
  if (str === undefined || str === null) {
    return undefined
  }
  const proxy = JSON.parse(str)
  if (proxy.id === undefined || proxy.id === null) {
    return undefined
  }
  return proxy
}
