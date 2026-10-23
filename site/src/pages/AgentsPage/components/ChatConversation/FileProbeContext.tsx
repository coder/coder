import { createContext, useContext, useRef, useState } from "react";
import type { AttachmentFailure } from "../../utils/chatAttachments";

type FileProbeContextValue = {
	hasExpired: (fileId: string) => boolean;
	isPending: (fileId: string) => boolean;
	markPending: (fileId: string) => void;
	clearPending: (fileId: string) => void;
	getProbeResult: (fileId: string) => AttachmentFailure | undefined;
	setProbeResult: (fileId: string, result: AttachmentFailure) => void;
};

const noFileProbes: FileProbeContextValue = {
	hasExpired: () => false,
	isPending: () => false,
	markPending: () => {},
	clearPending: () => {},
	getProbeResult: () => undefined,
	setProbeResult: () => {},
};

const FileProbeContext = createContext<FileProbeContextValue | undefined>(
	undefined,
);

/**
 * A provider inside another shares the outer one's probe results, so a file
 * that one view's probe finds deleted is expired in every view. Its own
 * evicted files are expired only for the views inside it.
 */
export const FileProbeProvider: React.FC<
	React.PropsWithChildren<{ evictedFileIds?: ReadonlySet<string> }>
> = ({ evictedFileIds, children }) => {
	const outerProbes = useContext(FileProbeContext);
	// Ref, not state: must be readable synchronously by the second
	// onError handler before React re-renders.
	const pendingProbeFileIds = useRef<Set<string>>(new Set());
	const [probeResults, setProbeResults] = useState<
		Map<string, AttachmentFailure>
	>(() => new Map());
	const setProbeResult = (fileId: string, result: AttachmentFailure) => {
		setProbeResults((prev) => new Map(prev).set(fileId, result));
	};

	const probes: FileProbeContextValue = outerProbes ?? {
		hasExpired: (fileId) => probeResults.get(fileId)?.kind === "expired",
		isPending: (fileId) => pendingProbeFileIds.current.has(fileId),
		markPending: (fileId) => {
			pendingProbeFileIds.current.add(fileId);
		},
		clearPending: (fileId) => {
			pendingProbeFileIds.current.delete(fileId);
		},
		getProbeResult: (fileId) => probeResults.get(fileId),
		setProbeResult,
	};

	return (
		<FileProbeContext.Provider
			value={{
				...probes,
				hasExpired: (fileId) =>
					evictedFileIds?.has(fileId) || probes.hasExpired(fileId),
			}}
		>
			{children}
		</FileProbeContext.Provider>
	);
};

/** The nearest provider's probes; outside any provider, they record nothing. */
export const useFileProbes = () => useContext(FileProbeContext) ?? noFileProbes;
