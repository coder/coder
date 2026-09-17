/**
 * Best-effort React component detection. React stores a fiber reference
 * on every host DOM node it renders (`__reactFiber$<random>`), and the
 * fiber tree exposes the owning component chain. Development builds of
 * React 18 and older also attach `_debugSource` with the JSX call site.
 */

interface FiberLike {
	type: unknown;
	return: FiberLike | null;
	_debugSource?: {
		fileName?: string;
		lineNumber?: number;
		columnNumber?: number;
	};
}

const skippedComponentNames = new Set([
	"Fragment",
	"Suspense",
	"StrictMode",
	"Profiler",
]);

function isFiber(value: unknown): value is FiberLike {
	return typeof value === "object" && value !== null && "return" in value;
}

// The page owns these objects and can make them cyclic or throwing, so
// every walk is depth-bounded and property reads are guarded.
const maxFiberDepth = 200;
const maxWrapperDepth = 5;
const maxNameLength = 100;

function fiberFor(element: Element): FiberLike | undefined {
	try {
		for (const key of Object.keys(element)) {
			if (
				key.startsWith("__reactFiber$") ||
				key.startsWith("__reactInternalInstance$")
			) {
				const candidate: unknown = Reflect.get(element, key);
				if (isFiber(candidate)) {
					return candidate;
				}
			}
		}
	} catch {
		// A throwing getter means no usable fiber.
	}
	return undefined;
}

function componentName(type: unknown, depth = 0): string | undefined {
	if (depth > maxWrapperDepth) {
		return undefined;
	}
	if (typeof type === "function") {
		const named: { displayName?: unknown; name?: unknown } = type;
		const display = named.displayName ?? named.name;
		return typeof display === "string" && display !== ""
			? display.slice(0, maxNameLength)
			: undefined;
	}
	// forwardRef and memo wrap the render function in an object.
	if (typeof type === "object" && type !== null) {
		const wrapped: { displayName?: unknown; render?: unknown; type?: unknown } =
			type;
		if (typeof wrapped.displayName === "string") {
			return wrapped.displayName.slice(0, maxNameLength);
		}
		return componentName(wrapped.render ?? wrapped.type, depth + 1);
	}
	return undefined;
}

interface ReactInfo {
	components: string[];
	sourceLocation?: string;
}

/**
 * Walks up from the element's fiber and returns the closest user-defined
 * component names (innermost first, at most `limit`) plus the nearest
 * JSX source location when the React build exposes it.
 */
export function describeReactOwner(
	element: Element,
	limit = 3,
): ReactInfo | undefined {
	let fiber: FiberLike | null | undefined = fiberFor(element);
	if (!fiber) {
		return undefined;
	}
	const components: string[] = [];
	let sourceLocation: string | undefined;
	const seen = new Set<FiberLike>();
	try {
		while (
			fiber &&
			components.length < limit &&
			seen.size < maxFiberDepth &&
			!seen.has(fiber)
		) {
			seen.add(fiber);
			const source = fiber._debugSource;
			if (!sourceLocation && typeof source?.fileName === "string") {
				const file = source.fileName.slice(0, maxNameLength * 3);
				sourceLocation =
					typeof source.lineNumber === "number"
						? `${file}:${source.lineNumber}`
						: file;
			}
			const name = componentName(fiber.type);
			if (
				name &&
				!skippedComponentNames.has(name) &&
				!name.endsWith("Provider") &&
				!name.endsWith("Consumer") &&
				components[components.length - 1] !== name
			) {
				components.push(name);
			}
			fiber = fiber.return;
		}
	} catch {
		// Partial results are still useful; the page threw mid-walk.
	}
	if (components.length === 0 && !sourceLocation) {
		return undefined;
	}
	return { components, sourceLocation };
}
