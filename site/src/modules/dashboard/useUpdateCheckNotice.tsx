import { SparkleIcon } from "lucide-react";
import { useEffect } from "react";
import { toast } from "sonner";
import { docs } from "#/utils/docs";

// A stable id keeps repeated renders from stacking duplicate toasts; sonner
// updates the existing toast in place instead of creating a new one.
const UPDATE_CHECK_TOAST_ID = "update-check-notice";

type UseUpdateCheckNoticeOptions = {
	/** Whether an update is available and has not been dismissed. */
	isVisible: boolean;
	version: string | undefined;
	releaseNotesUrl: string | undefined;
	/** Adds bottom margin so the toast clears the deployment banner. */
	aboveDeploymentBanner?: boolean;
	/** Persists the dismissal when the user closes the toast. */
	onDismiss: () => void;
};

/**
 * Surfaces an available Coder update through the shared Toaster. Showing a toast
 * is a side effect, so this is a hook rather than a component that renders null:
 * it drives a single persistent toast that stays until the user closes it with
 * the toast's close button, whose dismissal persists through `onDismiss`.
 *
 * The toast is only delivered once the shared `Toaster` is mounted; in the app
 * it mounts at the root long before the async update check resolves.
 */
export const useUpdateCheckNotice = ({
	isVisible,
	version,
	releaseNotesUrl,
	aboveDeploymentBanner = false,
	onDismiss,
}: UseUpdateCheckNoticeOptions) => {
	useEffect(() => {
		if (!isVisible || !version || !releaseNotesUrl) {
			return;
		}

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
						View release notes
					</a>
					<a
						href={docs("/install/upgrade")}
						target="_blank"
						rel="noreferrer"
						className="text-content-link"
					>
						View upgrade instructions
					</a>
				</span>
			),
			// sonner fires onDismiss for the close button but not for programmatic
			// dismissal, so closing persists the version while cleanup does not.
			onDismiss,
		});

		return () => {
			toast.dismiss(UPDATE_CHECK_TOAST_ID);
		};
	}, [isVisible, version, releaseNotesUrl, aboveDeploymentBanner, onDismiss]);
};
