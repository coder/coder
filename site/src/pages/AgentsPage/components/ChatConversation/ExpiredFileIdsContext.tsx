import {
	createContext,
	type FC,
	type PropsWithChildren,
	useContext,
	useRef,
	useState,
} from "react";

type ExpiredFileIdsContextValue = {
	hasExpired: (fileId: string) => boolean;
	markExpired: (fileId: string) => void;
	// isPending and markPending track in-flight probes so that when the same
	// file ID appears in multiple blocks, only the first onError handler starts
	// a network probe. The others fall through to the optimistic "failed" tile
	// and will be upgraded to "expired" if markExpired is later called.
	isPending: (fileId: string) => boolean;
	markPending: (fileId: string) => void;
};

const ExpiredFileIdsContext = createContext<ExpiredFileIdsContextValue>({
	hasExpired: () => false,
	markExpired: () => {},
	isPending: () => false,
	markPending: () => {},
});

export const ExpiredFileIdsProvider: FC<PropsWithChildren> = ({ children }) => {
	const [expiredFileIds, setExpiredFileIds] = useState<Set<string>>(
		() => new Set(),
	);
	// Use a ref so pending-state changes don't trigger re-renders; we only
	// need re-renders when expiredFileIds changes.
	const pendingProbeFileIds = useRef<Set<string>>(new Set());

	return (
		<ExpiredFileIdsContext.Provider
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
			}}
		>
			{children}
		</ExpiredFileIdsContext.Provider>
	);
};

export const useExpiredFileIds = () => useContext(ExpiredFileIdsContext);
