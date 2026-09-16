import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useEffect,
	useRef,
	useState,
} from "react";

interface ComposerHandle {
	// Attaches files to the draft; the user still reviews and sends.
	attach: (files: File[]) => void;
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
 * Lets right-panel tools (port preview annotations, for example) hand
 * files to the chat composer without threading callbacks through the page
 * tree. The composer registers its handle on mount.
 */
export const ComposerProvider: FC<{ children: ReactNode }> = ({ children }) => {
	// The latest handle lives in a ref so consumers get one stable object
	// and re-registration on every composer render does not cascade.
	const handleRef = useRef<ComposerHandle | null>(null);
	const [registered, setRegistered] = useState(false);
	const [stableHandle] = useState<ComposerHandle>(() => ({
		attach: (files) => handleRef.current?.attach(files),
	}));

	const register = (handle: ComposerHandle | null) => {
		handleRef.current = handle;
		setRegistered(handle !== null);
	};

	return (
		<ComposerContext
			value={{ composer: registered ? stableHandle : undefined, register }}
		>
			{children}
		</ComposerContext>
	);
};

export const useComposer = () => useContext(ComposerContext).composer;

/**
 * Registers the composer handle for the lifetime of the component. Call
 * from the component that owns attachment state.
 */
export const useRegisterComposer = (handle: ComposerHandle | null) => {
	const { register } = useContext(ComposerContext);
	useEffect(() => {
		register(handle);
		return () => register(null);
	}, [register, handle]);
};
