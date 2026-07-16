import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { cn } from "#/utils/cn";
import { isExternalImageSource } from "#/utils/externalImageSources";

export const MCPServerIcon: FC<{
	iconUrl: string;
	name: string;
	className?: string;
}> = ({ iconUrl, name, className }) => {
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded bg-surface-secondary border border-solid border-border",
				className,
			)}
		>
			{/* External icon URLs fall back to the generic icon so viewing
			    this server never discloses the viewer's IP to the icon
			    host (Cure53 CDM-02-006). New configs are validated
			    server-side; this also covers pre-validation rows. */}
			{iconUrl && !isExternalImageSource(iconUrl) ? (
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className="size-3/5"
				/>
			) : (
				<ServerIcon className="size-3/5 text-content-secondary" />
			)}
		</div>
	);
};
