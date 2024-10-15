import { CoderIcon } from "components/Icons/CoderIcon";
import type { FC } from "react";
import { getApplicationName, getLogoURL } from "utils/appearance";

/**
 * Enterprise customers can set a custom logo for their Coder application. Use
 * the custom logo wherever the Coder logo is used, if a custom one is provided.
 */
export const CustomLogo: FC = () => {
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
			css={{ maxWidth: 200 }}
			className="application-logo"
		/>
	) : (
		<CoderIcon css={{ fontSize: 64, fill: "white" }} />
	);
};
