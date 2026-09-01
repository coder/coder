import { SparkleIcon } from "lucide-react";
import type { FC } from "react";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { docs } from "#/utils/docs";

type UpdateCheckNoticeProps = {
	version: string;
	releaseNotesUrl: string;
	onDismiss: () => void;
	aboveDeploymentBanner?: boolean;
};

// A stable id keeps repeated renders from stacking duplicate toasts; sonner
// updates the existing toast instead of creating a new one.
const UPDATE_CHECK_TOAST_ID = "update-check-notice";

/**
 * Surfaces an available Coder update through the shared Toaster. It renders no
 * DOM itself; it drives a persistent toast that stays until the user closes it
 * with the toast's close button, whose dismissal persists through `onDismiss`.
 */
export const UpdateCheckNotice: FC<UpdateCheckNoticeProps> = ({
	version,
	releaseNotesUrl,
	onDismiss,
	aboveDeploymentBanner = false,
}) => {
	// Keep the latest onDismiss without re-running the effect when its identity
	// changes on each render of the parent.
	const onDismissRef = useRef(onDismiss);
	onDismissRef.current = onDismiss;

	useEffect(() => {
		toast(`Coder ${version} is now available`, {
			id: UPDATE_CHECK_TOAST_ID,
			duration: Number.POSITIVE_INFINITY,
			// A sparkle nods at the new release instead of the generic info icon.
			icon: <SparkleIcon className="text-content-primary" />,
			// Clear the deployment banner so the toast doesn't overlap it.
			className: aboveDeploymentBanner ? "mb-9" : undefined,
			description: (
				<span className="flex flex-col items-start">
					<a
						href={releaseNotesUrl}
						target="_blank"
						rel="noreferrer"
						className="text-content-link"
					>
						Release notes
					</a>
					<a
						href={docs("/install/upgrade")}
						target="_blank"
						rel="noreferrer"
						className="text-content-link"
					>
						Upgrade instructions
					</a>
				</span>
			),
			// The close button dismisses the toast; persist so it stays gone.
			onDismiss: () => onDismissRef.current(),
		});

		return () => {
			toast.dismiss(UPDATE_CHECK_TOAST_ID);
		};
	}, [version, releaseNotesUrl, aboveDeploymentBanner]);

	return null;
};
