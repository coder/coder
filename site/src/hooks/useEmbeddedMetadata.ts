import { useMemo, useSyncExternalStore } from "react";
import type {
  AppearanceConfig,
  BuildInfoResponse,
  Entitlements,
  Experiments,
  User,
} from "api/typesGenerated";

/**
 * This is the set of values that are currently being exposed to the React
 * application during production. These values are embedded via the Go server,
 * so they will never exist when using a JavaScript runtime for the backend
 *
 * If you want to add new metadata in a type-safe way, add it to this type.
 * Each key should the name of the "property" attribute that will be used on the
 * HTML elements themselves, and the values should be the data you get back from
 * parsing those element's text content
 */
type AvailableMetadata = Readonly<{
  user: User;
  experiments: Experiments;
  appearance: AppearanceConfig;
  entitlements: Entitlements;
  "build-info": BuildInfoResponse;
}>;

type MetadataKey = keyof AvailableMetadata;
export type MetadataValue = AvailableMetadata[MetadataKey];

export type MetadataState<T extends MetadataValue> = Readonly<{
  // undefined chosen to signify missing value because unlike null, it isn't a
  // valid JSON-serializable value. It's impossible to be returned by the API
  value: T | undefined;
  status: "missing" | "loaded" | "deleted";
}>;

export type RuntimeHtmlMetadata = Readonly<{
  [Key in MetadataKey]: MetadataState<AvailableMetadata[Key]>;
}>;

type SubscriptionCallback = (metadata: RuntimeHtmlMetadata) => void;
type QuerySelector = typeof document.querySelector;

type ParseJsonResult<T extends MetadataValue> = Readonly<
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
  private readonly querySelector: QuerySelector;
  private readonly subscriptions: Set<SubscriptionCallback>;
  private readonly trackedMetadataNodes: Map<string, Element | null>;
  private metadata: RuntimeHtmlMetadata;

  constructor(querySelector?: QuerySelector) {
    this.querySelector = querySelector ?? document.querySelector;
    this.subscriptions = new Set();
    this.trackedMetadataNodes = new Map();

    this.metadata = {
      user: this.registerValue<User>("user"),
      appearance: this.registerValue<AppearanceConfig>("appearance"),
      entitlements: this.registerValue<Entitlements>("entitlements"),
      experiments: this.registerValue<Experiments>("experiments"),
      "build-info": this.registerValue<BuildInfoResponse>("build-info"),
    };
  }

  private notifySubscriptionsOfStateChange(): void {
    const metadataBinding = this.metadata;
    this.subscriptions.forEach((cb) => cb(metadataBinding));
  }

  private registerValue<T extends MetadataValue>(
    key: MetadataKey,
  ): MetadataState<T> {
    const { value, node } = this.parseJson<T>(key);

    let newEntry: MetadataState<T>;
    if (!node || value === undefined) {
      newEntry = { value: undefined, status: "missing" };
    } else {
      newEntry = { value, status: "loaded" };
    }

    this.trackedMetadataNodes.set(key, node);
    return newEntry;
  }

  private parseJson<T extends MetadataValue>(key: string): ParseJsonResult<T> {
    const node = this.querySelector(`meta[property=${key}]`);
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
    if (metadataValue.status === "missing") {
      return;
    }

    const metadataNode = this.trackedMetadataNodes.get(key);
    this.trackedMetadataNodes.delete(key);

    // Delete the node entirely so that no other code can accidentally access
    // the value after it's supposed to have been made unavailable
    metadataNode?.remove();

    type NewState = MetadataState<NonNullable<typeof metadataValue.value>>;
    const newState: NewState = {
      ...metadataValue,
      value: undefined,
      status: "deleted",
    };

    this.metadata = { ...this.metadata, [key]: newState };
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
    // metadata object, meaning that even if you only care about one value,
    // using the hook will cause a component to re-render if the object changes
    // at all If this becomes a performance issue down the line, we can look
    // into selector functions to minimize re-renders
    const metadata = useSyncExternalStore(
      manager.subscribe,
      manager.getMetadata,
    );

    const stableMetadataResult = useMemo<UseEmbeddedMetadataResult>(() => {
      return {
        metadata,
        clearMetadataByKey: manager.clearMetadataByKey,
      };
    }, [metadata]);

    return stableMetadataResult;
  };
}

const defaultManager = new MetadataManager();
export const useEmbeddedMetadata = makeUseEmbeddedMetadata(defaultManager);
