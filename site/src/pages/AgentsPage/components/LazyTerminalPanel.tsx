import { lazy, Suspense, useEffect, useEffectEvent, useState } from "react";
import { Loader } from "#/components/Loader/Loader";

const TerminalPanel = lazy(() => import("./TerminalPanel"));

type LazyTerminalPanelProps = React.ComponentProps<typeof TerminalPanel>;

// A pending tab stays hidden until `onReady`, which would otherwise also wait
// for the chunk. Suspense shows this only while the chunk loads.
const TerminalPanelLoader: React.FC<{ onReady: () => void }> = ({
	onReady,
}) => {
	const signalReady = useEffectEvent(onReady);
	useEffect(() => {
		signalReady();
	}, []);
	return <Loader />;
};

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
		<Suspense fallback={<TerminalPanelLoader onReady={props.onReady} />}>
			<TerminalPanel {...props} />
		</Suspense>
	);
};
