import { useState } from "react";
import {
	type AttachmentFailure,
	attachmentFailureFromError,
	probeAttachmentFailure,
} from "../../utils/chatAttachments";
import { useFileProbes } from "./FileProbeContext";

export type AttachmentFailureState = { kind: "idle" } | AttachmentFailure;

/**
 * The first image of a file to fail probes it and shares the result with the
 * file's other images; a 404 also marks the file expired for download cards.
 */
export const useAttachmentFailure = (
	fileId: string | undefined,
	href: string,
): [failure: AttachmentFailureState, onError: () => void] => {
	const {
		hasExpired,
		markExpired,
		isPending,
		markPending,
		clearPending,
		getProbeResult,
		setProbeResult,
	} = useFileProbes();
	const [hasFailed, setHasFailed] = useState(false);

	const onError = () => {
		// Show the generic failure tile instead of the broken-image icon
		// until a probe result arrives.
		setHasFailed(true);
		// Inline (data:) images have no file ID to expire, and a pending
		// probe already reports to every image of the file.
		if (fileId === undefined || hasExpired(fileId) || isPending(fileId)) {
			return;
		}
		markPending(fileId);
		// No abort on unmount: the result must still reach the file's other
		// images and download cards.
		void probeAttachmentFailure(href)
			.then((reason) => {
				if (reason.kind === "expired") {
					markExpired(fileId);
				}
				setProbeResult(fileId, reason);
			})
			.catch((error) => {
				setProbeResult(fileId, attachmentFailureFromError(error));
			})
			.finally(() => {
				clearPending(fileId);
			});
	};

	if (fileId !== undefined && hasExpired(fileId)) {
		return [{ kind: "expired" }, onError];
	}
	const sharedResult = fileId ? getProbeResult(fileId) : undefined;
	const localFailure: AttachmentFailureState = hasFailed
		? { kind: "failed" }
		: { kind: "idle" };
	return [sharedResult ?? localFailure, onError];
};
