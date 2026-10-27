import { lazy, Suspense, useState } from "react";
import { Loader } from "#/components/Loader/Loader";

const TerminalPanel = lazy(() => import("./TerminalPanel"));

type LazyTerminalPanelProps = React.ComponentProps<typeof TerminalPanel>;

/**
 * Loads `TerminalPanel`, and with it xterm, when its tab is first hot: the
 * sidebar mounts every tab panel up front. The panel then stays mounted so
 * its idle detach still runs.
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
