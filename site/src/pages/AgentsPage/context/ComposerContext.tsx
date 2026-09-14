import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useEffect,
	useState,
} from "react";

interface ComposerHandle {
	// Sends a message as if the user had typed and submitted it.
	send: (message: string) => Promise<void> | void;
}

interface ComposerContextValue {
	composer: ComposerHandle | undefined;
	register: (handle: ComposerHandle | null) => void;
}

const ComposerContext = createContext<ComposerContextValue>({
	composer: undefined,
	register: () => undefined,
});

/**
 * Lets right-panel tools (port preview annotations, for example) submit
 * messages through the chat composer without threading callbacks through
 * the page tree. The composer registers its handle on mount.
 */
export const ComposerProvider: FC<{ children: ReactNode }> = ({ children }) => {
	const [composer, setComposer] = useState<ComposerHandle>();
	const register = (handle: ComposerHandle | null) => {
		setComposer(handle ?? undefined);
	};
	return (
		<ComposerContext value={{ composer, register }}>{children}</ComposerContext>
	);
};

export const useComposer = () => useContext(ComposerContext).composer;

/**
 * Registers the composer handle for the lifetime of the component. Call
 * from the component that owns message submission.
 */
export const useRegisterComposer = (handle: ComposerHandle | undefined) => {
	const { register } = useContext(ComposerContext);
	useEffect(() => {
		register(handle ?? null);
		return () => register(null);
	}, [register, handle]);
};
