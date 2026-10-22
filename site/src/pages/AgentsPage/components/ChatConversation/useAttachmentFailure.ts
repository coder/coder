import { useState } from "react";
import { useLatestAbortController } from "../../hooks/useLatestAbortController";
import {
	type AttachmentFailure,
	attachmentFailureFromError,
	isAbortError,
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
	const isKnownExpired = fileId !== undefined && hasExpired(fileId);
	const [failureState, setFailureState] = useState<AttachmentFailureState>({
		kind: "idle",
	});
	const probeRequest = useLatestAbortController(isKnownExpired);

	const onError = () => {
		// Inline (data:) images have no file ID to expire, so they go
		// straight to the generic failure tile.
		if (fileId === undefined) {
			setFailureState({ kind: "failed" });
			return;
		}
		if (hasExpired(fileId)) {
			setFailureState({ kind: "expired" });
			return;
		}
		// Dedup: skip probe, context will propagate the result.
		if (isPending(fileId)) {
			setFailureState({ kind: "failed" });
			return;
		}

		markPending(fileId);
		const controller = probeRequest.start();
		// Optimistically swap to the generic failure tile. The
		// probe will either upgrade it to "expired" or fill in
		// a detail; showing a tile without a label flash is
		// preferable to leaving the broken-image icon up.
		setFailureState({ kind: "failed" });

		void probeAttachmentFailure(href, controller.signal)
			.then((reason) => {
				clearPending(fileId);
				// Context writes stay above the clear() guard so a probe
				// that already resolved still reaches siblings after this
				// block unmounts or starts a newer probe.
				if (reason.kind === "expired") {
					markExpired(fileId);
				}
				setProbeResult(fileId, reason);
				if (probeRequest.clear(controller)) {
					setFailureState(reason);
				}
			})
			.catch((error) => {
				clearPending(fileId);
				if (isAbortError(error)) {
					return;
				}
				const failure = attachmentFailureFromError(error);
				setProbeResult(fileId, failure);
				if (probeRequest.clear(controller)) {
					setFailureState(failure);
				}
			});
	};

	if (isKnownExpired) {
		return [{ kind: "expired" }, onError];
	}
	const sharedResult = fileId ? getProbeResult(fileId) : undefined;
	return [sharedResult ?? failureState, onError];
};
