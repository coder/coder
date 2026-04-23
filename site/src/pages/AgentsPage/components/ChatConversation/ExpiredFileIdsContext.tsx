import {
	createContext,
	type FC,
	type PropsWithChildren,
	useContext,
	useState,
} from "react";

type ExpiredFileIdsContextValue = {
	hasExpired: (fileId: string) => boolean;
	markExpired: (fileId: string) => void;
};

const ExpiredFileIdsContext = createContext<ExpiredFileIdsContextValue>({
	hasExpired: () => false,
	markExpired: () => {},
});

export const ExpiredFileIdsProvider: FC<PropsWithChildren> = ({ children }) => {
	const [expiredFileIds, setExpiredFileIds] = useState<Set<string>>(
		() => new Set(),
	);

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
			}}
		>
			{children}
		</ExpiredFileIdsContext.Provider>
	);
};

export const useExpiredFileIds = () => useContext(ExpiredFileIdsContext);
