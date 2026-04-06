import type { FC } from "react";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import { getApplicationName, getLogoURL } from "#/utils/appearance";
import { cn } from "#/utils/cn";

/**
 * Enterprise customers can set a custom logo for their Coder application. Use
 * the custom logo wherever the Coder logo is used, if a custom one is provided.
 */
export const CustomLogo: FC<{ className?: string }> = ({ className }) => {
	const applicationName = getApplicationName();
	const logoURL = getLogoURL();

	return logoURL ? (
		<img
			alt={applicationName}
			src={logoURL}
			// This prevent browser to display the ugly error icon if the
			// image path is wrong or user didn't finish typing the url
			onError={(e) => {
				e.currentTarget.style.display = "none";
			}}
			onLoad={(e) => {
				e.currentTarget.style.display = "inline";
			}}
			className={cn("max-w-[200px] application-logo", className)}
		/>
	) : (
		<CoderIcon className={cn("w-12 h-12", className)} />
	);
};
