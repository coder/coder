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
	markProbing: (fileId: string) => boolean;
	clearProbing: (fileId: string) => void;
};

const ExpiredFileIdsContext = createContext<ExpiredFileIdsContextValue>({
	hasExpired: () => false,
	markExpired: () => {},
	markProbing: () => false,
	clearProbing: () => {},
});

export const ExpiredFileIdsProvider: FC<PropsWithChildren> = ({ children }) => {
	const [expiredFileIds, setExpiredFileIds] = useState<Set<string>>(
		() => new Set(),
	);
	const expiredFileIdsRef = useRef(expiredFileIds);
	const probingFileIdsRef = useRef<Set<string>>(new Set());

	return (
		<ExpiredFileIdsContext.Provider
			value={{
				hasExpired: (fileId) => expiredFileIdsRef.current.has(fileId),
				markExpired: (fileId) => {
					if (expiredFileIdsRef.current.has(fileId)) {
						probingFileIdsRef.current.delete(fileId);
						return;
					}
					const nextExpiredFileIds = new Set(expiredFileIdsRef.current);
					nextExpiredFileIds.add(fileId);
					expiredFileIdsRef.current = nextExpiredFileIds;
					setExpiredFileIds(nextExpiredFileIds);
					probingFileIdsRef.current.delete(fileId);
				},
				markProbing: (fileId) => {
					if (
						expiredFileIdsRef.current.has(fileId) ||
						probingFileIdsRef.current.has(fileId)
					) {
						return false;
					}
					probingFileIdsRef.current.add(fileId);
					return true;
				},
				clearProbing: (fileId) => {
					probingFileIdsRef.current.delete(fileId);
				},
			}}
		>
			{children}
		</ExpiredFileIdsContext.Provider>
	);
};

export const useExpiredFileIds = () => useContext(ExpiredFileIdsContext);
