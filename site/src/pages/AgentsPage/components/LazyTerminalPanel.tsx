import { lazy, Suspense, useState } from "react";
import { Loader } from "#/components/Loader/Loader";

const TerminalPanel = lazy(() => import("./TerminalPanel"));

type LazyTerminalPanelProps = React.ComponentProps<typeof TerminalPanel>;

/**
 * Loads `TerminalPanel`, and with it xterm, the first time its tab is hot.
 * The sidebar keeps every tab panel mounted, so a static import would load
 * xterm on every chat page. The panel stays mounted afterwards so its idle
 * detach keeps working.
 */
export const LazyTerminalPanel: React.FC<LazyTerminalPanelProps> = (props) => {
	const [wasHot, setWasHot] = useState(props.isHot);
	if (props.isHot && !wasHot) {
		setWasHot(true);
	}
	if (!wasHot) {
		return null;
	}
	return (
		<Suspense fallback={<Loader />}>
			<TerminalPanel {...props} />
		</Suspense>
	);
};
