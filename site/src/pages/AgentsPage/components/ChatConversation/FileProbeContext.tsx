import {
	createContext,
	type FC,
	type PropsWithChildren,
	useContext,
	useRef,
	useState,
} from "react";
import type { AttachmentFailure } from "../../utils/chatAttachments";

type FileProbeContextValue = {
	hasExpired: (fileId: string) => boolean;
	markExpired: (fileId: string) => void;
	isPending: (fileId: string) => boolean;
	markPending: (fileId: string) => void;
	clearPending: (fileId: string) => void;
	getProbeResult: (fileId: string) => AttachmentFailure | undefined;
	setProbeResult: (fileId: string, result: AttachmentFailure) => void;
};

const FileProbeContext = createContext<FileProbeContextValue>({
	hasExpired: () => false,
	markExpired: () => {},
	isPending: () => false,
	markPending: () => {},
	clearPending: () => {},
	getProbeResult: () => undefined,
	setProbeResult: () => {},
});

export const FileProbeProvider: FC<PropsWithChildren> = ({ children }) => {
	const [expiredFileIds, setExpiredFileIds] = useState<Set<string>>(
		() => new Set(),
	);
	// Ref, not state: must be readable synchronously by the second
	// onError handler before React re-renders.
	const pendingProbeFileIds = useRef<Set<string>>(new Set());
	const [probeResults, setProbeResults] = useState<
		Map<string, AttachmentFailure>
	>(() => new Map());

	return (
		<FileProbeContext.Provider
			value={{
				hasExpired: (fileId) => expiredFileIds.has(fileId),
				markExpired: (fileId) => {
					setExpiredFileIds((previous) => {
						if (previous.has(fileId)) {
							return previous;
						}
						const next = new Set(previous);
						next.add(fileId);
						return next;
					});
				},
				isPending: (fileId) => pendingProbeFileIds.current.has(fileId),
				markPending: (fileId) => {
					pendingProbeFileIds.current.add(fileId);
				},
				clearPending: (fileId) => {
					pendingProbeFileIds.current.delete(fileId);
				},
				getProbeResult: (fileId) => probeResults.get(fileId),
				setProbeResult: (fileId, result) => {
					setProbeResults((prev) => {
						const next = new Map(prev);
						next.set(fileId, result);
						return next;
					});
				},
			}}
		>
			{children}
		</FileProbeContext.Provider>
	);
};

export const useFileProbes = () => useContext(FileProbeContext);
