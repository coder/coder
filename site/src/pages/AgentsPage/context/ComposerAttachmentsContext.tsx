import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useEffect,
	useRef,
	useState,
} from "react";

type AttachHandler = (files: File[]) => void;

interface ComposerAttachmentsContextValue {
	// Attaches files to the active composer. No-op until a composer has
	// registered, so callers should check `canAttach` first.
	attach: AttachHandler;
	canAttach: boolean;
	register: (handler: AttachHandler | null) => void;
}

const ComposerAttachmentsContext =
	createContext<ComposerAttachmentsContextValue>({
		attach: () => undefined,
		canAttach: false,
		register: () => undefined,
	});

/**
 * Lets right-panel tools (port preview annotations, for example) hand files
 * to the chat composer without threading callbacks through the page tree.
 * The composer owns attachment state and registers its handler on mount.
 */
export const ComposerAttachmentsProvider: FC<{ children: ReactNode }> = ({
	children,
}) => {
	const handlerRef = useRef<AttachHandler | null>(null);
	const [canAttach, setCanAttach] = useState(false);

	const register = (handler: AttachHandler | null) => {
		handlerRef.current = handler;
		setCanAttach(handler !== null);
	};

	const attach: AttachHandler = (files) => {
		handlerRef.current?.(files);
	};

	return (
		<ComposerAttachmentsContext value={{ attach, canAttach, register }}>
			{children}
		</ComposerAttachmentsContext>
	);
};

export const useComposerAttachments = () =>
	useContext(ComposerAttachmentsContext);

/**
 * Registers the composer's attach handler for the lifetime of the
 * component. Call from the component that owns attachment state.
 */
export const useRegisterComposerAttachments = (
	handler: AttachHandler | undefined,
) => {
	const { register } = useComposerAttachments();
	useEffect(() => {
		register(handler ?? null);
		return () => register(null);
	}, [register, handler]);
};
