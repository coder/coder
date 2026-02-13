import {
	createContext,
	useCallback,
	useContext,
	useMemo,
	useState,
	type FC,
	type PropsWithChildren,
} from "react";
import { ChangelogDialog } from "./ChangelogDialog";

interface ChangelogContextValue {
	openChangelog: (version: string) => void;
}

const ChangelogContext = createContext<ChangelogContextValue | undefined>(
	undefined,
);

export const useChangelog = (): ChangelogContextValue => {
	const ctx = useContext(ChangelogContext);
	if (!ctx) {
		throw new Error("useChangelog must be used within ChangelogProvider");
	}
	return ctx;
};

export const ChangelogProvider: FC<PropsWithChildren> = ({ children }) => {
	const [version, setVersion] = useState<string | null>(null);

	const openChangelog = useCallback((v: string) => {
		setVersion(v);
	}, []);

	const closeChangelog = useCallback(() => {
		setVersion(null);
	}, []);

	const value = useMemo(() => ({ openChangelog }), [openChangelog]);

	return (
		<ChangelogContext.Provider value={value}>
			{children}
			<ChangelogDialog version={version} onClose={closeChangelog} />
		</ChangelogContext.Provider>
	);
};
