/**
 * Shared types used by multiple components go here.
 */

 import { RouteConfig } from "../../router"

 export interface NavbarEntryProps extends Pick<RouteConfig, "description" | "featureFlag" | "Icon" | "label" | "path"> {
   selected: boolean
   className?: string
   onClick?: () => void
 }
