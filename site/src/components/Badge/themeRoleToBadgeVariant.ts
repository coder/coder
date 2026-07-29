import type { ThemeRole } from "#/theme/roles";
import type { BadgeProps } from "./Badge";

/**
 * Maps a `ThemeRole` (or the legacy `muted` role) to the closest Badge
 * variant. Use this when a call site receives a `ThemeRole` rather than a
 * hardcoded Badge variant.
 */
export function themeRoleToBadgeVariant(
	role: ThemeRole | "muted",
): NonNullable<BadgeProps["variant"]> {
	switch (role) {
		case "success":
			return "green";
		case "error":
			return "destructive";
		case "warning":
		case "danger":
			return "warning";
		case "active":
		case "notice":
			return "info";
		case "preview":
			return "purple";
		default:
			return "default";
	}
}
