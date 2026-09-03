import { InfoIcon, XIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";

type UpdateCheckNoticeProps = {
	version: string;
	releaseNotesUrl: string;
	onDismiss: () => void;
	aboveDeploymentBanner?: boolean;
};

export const UpdateCheckNotice: FC<UpdateCheckNoticeProps> = ({
	version,
	releaseNotesUrl,
	onDismiss,
	aboveDeploymentBanner = false,
}) => {
	return (
		<div
			data-testid="update-check-notice"
			role="status"
			className={cn(
				"fixed right-6 z-50 flex max-w-[420px] items-start gap-4 rounded border border-solid border-highlight-sky bg-surface-primary p-4 text-sm text-content-primary shadow-sm",
				// 60px keeps a 24px gap above the 36px deployment banner.
				aboveDeploymentBanner ? "bottom-[60px]" : "bottom-6",
			)}
		>
			<InfoIcon className="mt-0.5 size-icon-sm shrink-0 text-highlight-sky" />
			<div className="flex flex-col gap-1">
				<p className="m-0 font-semibold">Coder {version} is now available. </p>
				<p className="m-0 flex-1 leading-5">
					View the{" "}
					<a
						href={releaseNotesUrl}
						target="_blank"
						rel="noreferrer"
						className="text-content-link underline hover:no-underline"
					>
						release notes
					</a>{" "}
					and{" "}
					<a
						href={docs("/install/upgrade")}
						target="_blank"
						rel="noreferrer"
						className="text-content-link underline hover:no-underline"
					>
						upgrade instructions
					</a>{" "}
					for more information.
				</p>
			</div>
			<Button
				aria-label="Dismiss"
				size="icon"
				variant="subtle"
				onClick={onDismiss}
				className="-m-2"
			>
				<XIcon />
			</Button>
		</div>
	);
};
