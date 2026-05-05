import type { FC } from "react";
import { getApplicationName, getLogoURL } from "#/utils/appearance";
import { cn } from "#/utils/cn";
import { ExternalImage } from "../ExternalImage/ExternalImage";

/**
 * Enterprise customers can set a custom logo for their Coder application. Use
 * the custom logo wherever the Coder logo is used, if a custom one is provided.
 */
export const ProductLogo: FC<{ className?: string }> = ({ className }) => {
	const applicationName = getApplicationName();
	const logoURL = getLogoURL();

	return logoURL ? (
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
	) : (
		<CoderLogo className={cn("h-12", className)} />
	);
};

const CoderLogo: FC<React.ComponentProps<"svg">> = ({
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
		<path d="M34.5381 0C54.5335 6.29882e-05 65.7432 10.1355 66.122 25.0544L48.853 25.6216C48.3984 17.3514 41.544 11.9189 34.5381 12.0809C24.919 12.2836 17.7989 19.1351 17.7988 29.9999C17.7988 40.8648 24.919 47.5951 34.5381 47.5951C41.544 47.5945 48.2468 42.4055 49.0043 34.1352L66.2733 34.5408C65.8189 49.7027 53.9276 60 34.5381 60C15.1484 60 0 48.2433 0 29.9999C7.1014e-05 11.6757 14.5426 0 34.5381 0ZM120 1.7728V58.5299H74.5559V1.7728H120Z" />
	</svg>
);
