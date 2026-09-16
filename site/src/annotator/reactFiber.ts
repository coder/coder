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

function fiberFor(element: Element): FiberLike | undefined {
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
	return undefined;
}

function componentName(type: unknown): string | undefined {
	if (typeof type === "function") {
		const named: { displayName?: unknown; name?: unknown } = type;
		const display = named.displayName ?? named.name;
		return typeof display === "string" && display !== "" ? display : undefined;
	}
	// forwardRef and memo wrap the render function in an object.
	if (typeof type === "object" && type !== null) {
		const wrapped: { displayName?: unknown; render?: unknown; type?: unknown } =
			type;
		if (typeof wrapped.displayName === "string") {
			return wrapped.displayName;
		}
		return componentName(wrapped.render ?? wrapped.type);
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
	while (fiber && components.length < limit) {
		if (!sourceLocation && fiber._debugSource?.fileName) {
			const { fileName, lineNumber } = fiber._debugSource;
			sourceLocation =
				lineNumber === undefined ? fileName : `${fileName}:${lineNumber}`;
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
	if (components.length === 0 && !sourceLocation) {
		return undefined;
	}
	return { components, sourceLocation };
}
