import type { SvgIconProps } from "@mui/material/SvgIcon";
import type { FC } from "react";
import { cn } from "utils/cn";

/**
 * CoderIcon represents the cloud with brackets Coder brand icon. It does not
 * contain additional aspects, like the word 'Coder'.
 */
export const CoderIcon: FC<SvgIconProps> = ({ className, ...props }) => (
	<svg
		// This is a case where prop order does matter. We want fill to be easy
		// to override, but all other local props should stay locked down
		fill="currentColor"
		{...props}
		className={cn("w-14 h-7 text-content-primary", className)}
		viewBox="0 0 120 60"
		xmlns="http://www.w3.org/2000/svg"
	>
		<title>Coder logo</title>
		<path d="M34.5381 0C54.5335 6.29882e-05 65.7432 10.1355 66.122 25.0544L48.853 25.6216C48.3984 17.3514 41.544 11.9189 34.5381 12.0809C24.919 12.2836 17.7989 19.1351 17.7988 29.9999C17.7988 40.8648 24.919 47.5951 34.5381 47.5951C41.544 47.5945 48.2468 42.4055 49.0043 34.1352L66.2733 34.5408C65.8189 49.7027 53.9276 60 34.5381 60C15.1484 60 0 48.2433 0 29.9999C7.1014e-05 11.6757 14.5426 0 34.5381 0ZM120 1.7728V58.5299H74.5559V1.7728H120Z" />
	</svg>
);
