import { createContext, useContext, useState } from "react";

type RenderLightbox = (close: () => void) => React.ReactNode;

type OpenLightbox = { render: RenderLightbox; close: () => void };

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
		lightbox: open && { render: open.render, close: () => setOpen(null) },
	};
};

/** Renders the lightbox {@link useLightbox} returned, if one is open. */
export const LightboxSlot: React.FC<{ lightbox: OpenLightbox | null }> = ({
	lightbox,
}) => lightbox?.render(lightbox.close);

/**
 * Renders the lightbox a descendant opens with {@link useLightbox}, so it
 * stays open while the content that opened it unmounts.
 */
export const LightboxHost: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	const { openLightbox, lightbox } = useLightboxSlot();
	return (
		<LightboxHostContext value={openLightbox}>
			{children}
			<LightboxSlot lightbox={lightbox} />
		</LightboxHostContext>
	);
};

/**
 * Opens a lightbox in the nearest {@link LightboxHost}. Outside one, the
 * caller renders the returned `lightbox` in a {@link LightboxSlot}.
 */
export const useLightbox = () => {
	const openInHost = useContext(LightboxHostContext);
	const own = useLightboxSlot();
	return openInHost ? { openLightbox: openInHost, lightbox: null } : own;
};
