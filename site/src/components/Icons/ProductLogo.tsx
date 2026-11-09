import { cn } from "cn";
import { getApplicationName, getLogoURL } from "#/utils/appearance";
import { ExternalImage } from "../ExternalImage/ExternalImage";
import { CODER_LOGO_PATH } from "./coderLogoPath";
import { GhostLogo } from "./GhostLogo";

type ProductLogoProps = {
	className?: string;
	ghost?: boolean;
};

/**
 * Enterprise customers can set a custom logo for their Coder application. Use
 * the custom logo wherever the Coder logo is used, if a custom one is provided.
 *
 * `ghost` swaps the Coder logo for the GhostLogo hover easter egg. A custom logo
 * always wins: it's never animated.
 */
export const ProductLogo: React.FC<ProductLogoProps> = ({
	className,
	ghost = false,
}) => {
	const applicationName = getApplicationName();
	const logoURL = getLogoURL();

	if (logoURL) {
		return (
			<ExternalImage
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
				className={cn("h-12 max-w-[200px] application-logo", className)}
			/>
		);
	}
	if (ghost) {
		return <GhostLogo className={cn("h-12", className)} />;
	}
	return <CoderLogo className={cn("h-12", className)} />;
};

const CoderLogo: React.FC<React.ComponentProps<"svg">> = ({
	className,
	...props
}) => (
	<svg
		// This is a case where prop order does matter. We want fill to be easy
		// to override, but all other local props should stay locked down
		fill="currentColor"
		{...props}
		className={cn("h-7 aspect-square text-content-primary", className)}
		viewBox="0 0 120 60"
		xmlns="http://www.w3.org/2000/svg"
	>
		<title>Coder logo</title>
		<path d={CODER_LOGO_PATH} />
	</svg>
);
