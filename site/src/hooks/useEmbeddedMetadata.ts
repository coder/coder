import type {
	AppearanceConfig,
	BuildInfoResponse,
	Entitlements,
	Experiments,
	Region,
	User,
	UserAppearanceSettings,
} from "api/typesGenerated";
import { useMemo, useSyncExternalStore } from "react";

export const DEFAULT_METADATA_KEY = "property";

/**
 * This is the set of values that are currently being exposed to the React
 * application during production. These values are embedded via the Go server,
 * so they will never exist when using a JavaScript runtime for the backend
 *
 * If you want to add new metadata in a type-safe way, add it to this type.
 * Each key should be the name of the "property" attribute that will be used on
 * the HTML meta elements themselves (e.g., meta[property=${key}]), and the
 * values should be the data you get back from parsing those element's content
 * attributes
 */
type AvailableMetadata = Readonly<{
	user: User;
	experiments: Experiments;
	appearance: AppearanceConfig;
	userAppearance: UserAppearanceSettings;
	entitlements: Entitlements;
	regions: readonly Region[];
	"build-info": BuildInfoResponse;
}>;

export type MetadataKey = keyof AvailableMetadata;
export type MetadataValue = AvailableMetadata[MetadataKey];

export type MetadataState<T extends MetadataValue> = Readonly<{
	// undefined chosen to signify missing value because unlike null, it isn't a
	// valid JSON-serializable value. It's impossible to be returned by the API
	value: T | undefined;
	available: boolean;
}>;

const unavailableState = {
	value: undefined,
	available: false,
} as const satisfies MetadataState<MetadataValue>;

export type RuntimeHtmlMetadata = Readonly<{
	[Key in MetadataKey]: MetadataState<AvailableMetadata[Key]>;
}>;

type SubscriptionCallback = (metadata: RuntimeHtmlMetadata) => void;

type ParseJsonResult<T = unknown> = Readonly<
	| {
			value: T;
			node: Element;
	  }
	| {
			value: undefined;
			node: null;
	  }
>;

interface MetadataManagerApi {
	subscribe: (callback: SubscriptionCallback) => () => void;
	getMetadata: () => RuntimeHtmlMetadata;
	clearMetadataByKey: (key: MetadataKey) => void;
}

export class MetadataManager implements MetadataManagerApi {
	private readonly metadataKey: string;
	private readonly subscriptions: Set<SubscriptionCallback>;
	private readonly trackedMetadataNodes: Map<string, Element | null>;

	private metadata: RuntimeHtmlMetadata;

	constructor(metadataKey?: string) {
		this.metadataKey = metadataKey ?? DEFAULT_METADATA_KEY;
		this.subscriptions = new Set();
		this.trackedMetadataNodes = new Map();

		this.metadata = {
			user: this.registerValue<User>("user"),
			appearance: this.registerValue<AppearanceConfig>("appearance"),
			userAppearance:
				this.registerValue<UserAppearanceSettings>("userAppearance"),
			entitlements: this.registerValue<Entitlements>("entitlements"),
			experiments: this.registerValue<Experiments>("experiments"),
			"build-info": this.registerValue<BuildInfoResponse>("build-info"),
			regions: this.registerRegionValue(),
		};
	}

	private notifySubscriptionsOfStateChange(): void {
		const metadataBinding = this.metadata;
		for (const cb of this.subscriptions) {
			cb(metadataBinding);
		}
	}

	/**
	 * This is a band-aid solution for code that was specific to the Region
	 * type.
	 *
	 * Ideally the code should be updated on the backend to ensure that the
	 * response is one consistent type, and then this method should be removed
	 * entirely.
	 *
	 * Removing this method would also ensure that the other types in this file
	 * can be tightened up even further (e.g., adding a type constraint to
	 * parseJson)
	 */
	private registerRegionValue(): MetadataState<readonly Region[]> {
		type RegionResponse =
			| readonly Region[]
			| Readonly<{
					regions: readonly Region[];
			  }>;

		const { value, node } = this.parseJson<RegionResponse>("regions");

		let newEntry: MetadataState<readonly Region[]>;
		if (!node || value === undefined) {
			newEntry = unavailableState;
		} else if ("regions" in value) {
			newEntry = { value: value.regions, available: true };
		} else {
			newEntry = { value, available: true };
		}

		const key = "regions" satisfies MetadataKey;
		this.trackedMetadataNodes.set(key, node);
		return newEntry;
	}

	private registerValue<T extends MetadataValue>(
		key: MetadataKey,
	): MetadataState<T> {
		const { value, node } = this.parseJson<T>(key);

		let newEntry: MetadataState<T>;
		if (!node || value === undefined) {
			newEntry = unavailableState;
		} else {
			newEntry = { value, available: true };
		}

		this.trackedMetadataNodes.set(key, node);
		return newEntry;
	}

	private parseJson<T = unknown>(key: string): ParseJsonResult<T> {
		const node = document.querySelector(`meta[${this.metadataKey}=${key}]`);
		if (!node) {
			return { value: undefined, node: null };
		}

		const rawContent = node.getAttribute("content");
		if (rawContent) {
			try {
				const value = JSON.parse(rawContent) as T;
				return { value, node };
			} catch (err) {
				// In development, the metadata is always going to be empty; error is
				// only a concern for production
				if (process.env.NODE_ENV === "production") {
					console.warn(`Failed to parse ${key} metadata. Error message:`);
					console.warn(err);
				}
			}
		}

		return { value: undefined, node: null };
	}

	//////////////////////////////////////////////////////////////////////////////
	// All public functions should be defined as arrow functions to ensure that
	// they cannot lose their `this` context when passed around the React UI
	//////////////////////////////////////////////////////////////////////////////

	subscribe = (callback: SubscriptionCallback): (() => void) => {
		this.subscriptions.add(callback);
		return () => this.subscriptions.delete(callback);
	};

	getMetadata = (): RuntimeHtmlMetadata => {
		return this.metadata;
	};

	clearMetadataByKey = (key: MetadataKey): void => {
		const metadataValue = this.metadata[key];
		if (!metadataValue.available) {
			return;
		}

		const metadataNode = this.trackedMetadataNodes.get(key);
		this.trackedMetadataNodes.delete(key);

		// Delete the node entirely so that no other code can accidentally access
		// the value after it's supposed to have been made unavailable
		metadataNode?.remove();

		this.metadata = { ...this.metadata, [key]: unavailableState };
		this.notifySubscriptionsOfStateChange();
	};
}

type UseEmbeddedMetadataResult = Readonly<{
	metadata: RuntimeHtmlMetadata;
	clearMetadataByKey: MetadataManager["clearMetadataByKey"];
}>;

export function makeUseEmbeddedMetadata(
	manager: MetadataManager,
): () => UseEmbeddedMetadataResult {
	return function useEmbeddedMetadata(): UseEmbeddedMetadataResult {
		// Hook binds re-renders to the memory reference of the entire exposed
		// metadata object, meaning that even if you only care about one metadata
		// property, the hook will cause a component to re-render if the object
		// changes at all. If this becomes a performance issue down the line, we can
		// look into selector functions to minimize re-renders, but let's wait
		const metadata = useSyncExternalStore(
			manager.subscribe,
			manager.getMetadata,
		);

		// biome-ignore lint/correctness/useExhaustiveDependencies(manager.clearMetadataByKey): baked into containing hook
		const stableMetadataResult = useMemo<UseEmbeddedMetadataResult>(() => {
			return {
				metadata,
				clearMetadataByKey: manager.clearMetadataByKey,
			};
		}, [metadata]);

		return stableMetadataResult;
	};
}

export const defaultMetadataManager = new MetadataManager();
export const useEmbeddedMetadata = makeUseEmbeddedMetadata(
	defaultMetadataManager,
);
