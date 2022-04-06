import SvgIcon from "@material-ui/core/SvgIcon"
import { RouteProps } from "react-router-dom"

export type RouteNavbarGroup = "main" | "admin"

interface BasicRouteConfig {
  path: string
  /** Use to match when the path includes variables. */
  re?: RegExp
  component: RouteProps["component"]
  enabled: boolean
  label?: string
  Icon?: typeof SvgIcon
  group?: RouteNavbarGroup
  hideNavbar?: boolean
  description?: string
}

interface RouteConfigWithNav extends BasicRouteConfig {
  label: string
  icon: typeof SvgIcon
  group: RouteNavbarGroup
}

export type RouteConfig = BasicRouteConfig | RouteConfigWithNav
export interface NavbarEntryProps extends Pick<RouteConfig, "description" | "Icon" | "label" | "path"> {
  selected: boolean
  className?: string
  onClick?: () => void
}
