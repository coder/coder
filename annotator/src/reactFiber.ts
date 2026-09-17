/**
 * React component detection built on bippy. bippy reads the fiber React
 * attaches to every host DOM node and, in development builds, resolves
 * the JSX call sites React records on fibers back through source maps.
 * See reactShim.ts for how the bundle avoids pulling React in.
 */
import {
	type Fiber,
	getDisplayName,
	getFiberFromHostInstance,
	getLatestFiber,
	isCompositeFiber,
	traverseFiber,
} from "bippy";
import { getOwnerStack, getParentStack, getSource } from "bippy/source";

const skippedComponentNames = new Set([
	"Fragment",
	"Suspense",
	"StrictMode",
	"Profiler",
]);

// Props whose values are functions or React nodes are still worth
// naming; only the names are reported, never the values.
const skippedPropNames = new Set(["children", "key", "ref"]);
const maxProps = 20;
const maxOwnerFrames = 6;

interface ReactInfo {
	components: string[];
	// Prop names of the nearest component, in declaration order.
	props?: string[];
}

export interface ReactSource {
	// Where the nearest component is rendered from, `file:line:col`.
	sourceLocation?: string;
	// Components that created this element's JSX, outermost last, each
	// with its own call site when one could be resolved.
	reactOwnerStack?: string[];
}

function componentNameOf(fiber: Fiber): string | undefined {
	if (!isCompositeFiber(fiber)) {
		return undefined;
	}
	const name = getDisplayName(fiber.type);
	if (
		!name ||
		skippedComponentNames.has(name) ||
		name.endsWith("Provider") ||
		name.endsWith("Consumer")
	) {
		return undefined;
	}
	return name;
}

function nearestComposite(element: Element): Fiber | null {
	const host = getFiberFromHostInstance(element);
	if (!host) {
		return null;
	}
	return traverseFiber(
		getLatestFiber(host),
		(fiber) => componentNameOf(fiber) !== undefined,
		true,
	);
}

function propNames(fiber: Fiber): string[] | undefined {
	const props: unknown = fiber.memoizedProps;
	if (typeof props !== "object" || props === null) {
		return undefined;
	}
	const names = Object.keys(props)
		.filter((name) => !skippedPropNames.has(name))
		.slice(0, maxProps);
	return names.length > 0 ? names : undefined;
}

/**
 * Synchronous part: the closest user-defined component names (innermost
 * first, at most `limit`) and the nearest component's prop names.
 */
export function describeReactOwner(
	element: Element,
	limit = 3,
): ReactInfo | undefined {
	const nearest = nearestComposite(element);
	if (!nearest) {
		return undefined;
	}
	const components: string[] = [];
	traverseFiber(
		nearest,
		(fiber) => {
			const name = componentNameOf(fiber);
			if (name && components[components.length - 1] !== name) {
				components.push(name);
			}
			return components.length >= limit;
		},
		true,
	);
	return { components, props: propNames(nearest) };
}

function formatLocation(frame: {
	fileName?: string;
	lineNumber?: number;
	columnNumber?: number;
}): string | undefined {
	if (!frame.fileName) {
		return undefined;
	}
	const file = frame.fileName.replace(/^webpack:\/\/\/?|^\/@fs\//, "");
	if (frame.lineNumber === undefined) {
		return file;
	}
	return frame.columnNumber === undefined
		? `${file}:${frame.lineNumber}`
		: `${file}:${frame.lineNumber}:${frame.columnNumber}`;
}

/**
 * Asynchronous part: resolves the nearest component's render site and its
 * owner chain through source maps. Only development React builds carry
 * the debug data this needs; production builds yield nothing. Network
 * fetches for source maps are bounded by `timeoutMs`.
 */
export async function resolveReactSource(
	element: Element,
	timeoutMs = 1500,
): Promise<ReactSource | undefined> {
	const host = getFiberFromHostInstance(element);
	if (!host) {
		return undefined;
	}
	// The host fiber's own creation site is the JSX for this very element
	// (React 19); fall back to where the nearest component is rendered.
	const target = getLatestFiber(host);
	const timeout = new Promise<undefined>((resolve) =>
		setTimeout(() => resolve(undefined), timeoutMs),
	);
	const resolved = await Promise.race([
		Promise.all([
			getSource(target)
				.then((source) => {
					if (source) {
						return source;
					}
					const nearest = nearestComposite(element);
					return nearest ? getSource(nearest) : null;
				})
				.catch(() => null),
			getOwnerStack(target)
				.then((owners) => (owners.length > 0 ? owners : getParentStack(target)))
				.catch(() => []),
		]),
		timeout,
	]);
	if (!resolved) {
		return undefined;
	}
	const [source, owners] = resolved;
	// Frames from bundler-ignore-listed code (frameworks, node_modules)
	// and frames without a file are noise for locating app code.
	const ownerStack = owners
		.filter((frame) => !frame.isIgnoreListed && frame.fileName)
		.slice(0, maxOwnerFrames)
		.map((frame) => {
			const location = formatLocation(frame);
			const name = frame.functionName ?? "(anonymous)";
			return location ? `${name} (${location})` : name;
		});
	const sourceLocation = source ? formatLocation(source) : undefined;
	if (!sourceLocation && ownerStack.length === 0) {
		return undefined;
	}
	return {
		sourceLocation,
		reactOwnerStack: ownerStack.length > 0 ? ownerStack : undefined,
	};
}
