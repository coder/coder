import { createContext, useContext, useState } from "react";

type RenderLightbox = (close: () => void) => React.ReactNode;

const LightboxHostContext = createContext<
	((render: RenderLightbox) => void) | undefined
>(undefined);

/**
 * Holds one lightbox: opening one while another is open does nothing, so a
 * text preview whose fetch resolves late cannot replace one opened since.
 */
const useLightboxSlot = () => {
	const [open, setOpen] = useState<{ render: RenderLightbox } | null>(null);
	return {
		openLightbox: (render: RenderLightbox) =>
			setOpen((current) => current ?? { render }),
		lightbox: open?.render(() => setOpen(null)),
	};
};

/**
 * Renders the lightbox a descendant opens with {@link useLightbox}, so it
 * stays open while the content that opened it unmounts, as when a finished
 * turn folds into one collapsed block.
 */
export const LightboxHost: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	const { openLightbox, lightbox } = useLightboxSlot();
	return (
		<LightboxHostContext value={openLightbox}>
			{children}
			{lightbox}
		</LightboxHostContext>
	);
};

/**
 * Opens a lightbox in the nearest {@link LightboxHost}. Outside one, the
 * caller renders the returned `lightbox` itself.
 */
export const useLightbox = () => {
	const openInHost = useContext(LightboxHostContext);
	const own = useLightboxSlot();
	return openInHost ? { openLightbox: openInHost, lightbox: null } : own;
};
