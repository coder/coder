import { lazy, Suspense, useState } from "react";
import { Loader } from "#/components/Loader/Loader";
import { prefetchChunk } from "#/utils/staleChunkReload";

const loadTerminalPanel = () => import("./TerminalPanel");
let prefetch: Promise<void> | undefined;
// A load during the prefetch would skip the stale-chunk reload with it.
const TerminalPanel = lazy(async () => {
	await prefetch;
	return loadTerminalPanel();
});

/**
 * Starts loading `TerminalPanel` before a terminal tab opens. A new tab stays
 * hidden until its terminal is ready, so its first open would otherwise also
 * wait for the chunk.
 */
export const preloadTerminalPanel = (): void => {
	prefetch ??= prefetchChunk(loadTerminalPanel);
};

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
