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
 * If you need to add a new type of metadata value, add a new property to the
 * type alias here, and then rest of the file should light up with errors for
 * what else needs to be adjusted
 */
type SourceHtmlMetadata = Readonly<{
  user: User;
  experiments: Experiments;
  appearanceConfig: AppearanceConfig;
  buildInfo: BuildInfoResponse;
  entitlements: Entitlements;
}>;

type MetadataKey = keyof SourceHtmlMetadata;
export type RuntimeHtmlMetadata = Readonly<{
  [Key in MetadataKey]: SourceHtmlMetadata[Key] | undefined;
}>;

type SubscriptionCallback = (metadata: RuntimeHtmlMetadata) => void;
type QuerySelector = typeof document.querySelector;

interface MetadataManagerApi {
  subscribe: (callback: SubscriptionCallback) => () => void;
  getMetadata: () => RuntimeHtmlMetadata;
  clearMetadataByKey: (key: MetadataKey) => void;
}

export class MetadataManager implements MetadataManagerApi {
  private readonly querySelector: QuerySelector;
  private readonly subscriptions: Set<SubscriptionCallback>;
  private readonly trackedMetadataNodes: Map<string, Element>;
  private metadata: RuntimeHtmlMetadata;

  constructor(querySelector?: QuerySelector) {
    this.querySelector = querySelector ?? document.querySelector;
    this.subscriptions = new Set();
    this.trackedMetadataNodes = new Map();

    this.metadata = {
      user: this.registerAsJson<User>("user"),
      appearanceConfig: this.registerAsJson<AppearanceConfig>("appearance"),
      buildInfo: this.registerAsJson<BuildInfoResponse>("build-info"),
      entitlements: this.registerAsJson<Entitlements>("entitlements"),
      experiments: this.registerAsJson<Experiments>("experiments"),
    };
  }

  private notifySubscriptionsOfStateChange(): void {
    const metadataBinding = this.metadata;
    this.subscriptions.forEach((cb) => cb(metadataBinding));
  }

  private registerAsJson<T extends NonNullable<unknown>>(
    key: string,
  ): T | undefined {
    const metadataNode = this.querySelector(`meta[property=${key}]`);
    if (!metadataNode) {
      return undefined;
    }

    const rawContent = metadataNode.getAttribute("content");
    if (rawContent) {
      try {
        const result = JSON.parse(rawContent) as T;
        this.trackedMetadataNodes.set(key, metadataNode);
        return result;
      } catch (err) {
        // In development, the metadata is always going to be empty; error is
        // only a concern for production
        if (process.env.NODE_ENV === "production") {
          console.warn(`Failed to parse ${key} metadata. Error message:`);
          console.warn(err);
        }
      }
    }

    return undefined;
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
    if (metadataValue === undefined) {
      return;
    }

    const metadataNode = this.trackedMetadataNodes.get(key);
    metadataNode?.remove();
    this.trackedMetadataNodes.delete(key);

    this.metadata = {
      ...this.metadata,
      [key]: undefined,
    };

    this.notifySubscriptionsOfStateChange();
  };
}

type UseHtmlMetadataResult = Readonly<{
  metadata: RuntimeHtmlMetadata;
  clearMetadataByKey: MetadataManager["clearMetadataByKey"];
}>;

export function makeUseHtmlMetadata(
  manager: MetadataManager,
): () => UseHtmlMetadataResult {
  return function useHtmlMetadata(): UseHtmlMetadataResult {
    // Hook binds re-renders to the memory reference of the entire exposed
    // metadata object, meaning that even if you only care about one value,
    // using the hook will cause a component to re-render if the object changes
    // at all If this becomes a performance issue down the line, we can look
    // into selector functions to minimize re-renders
    const metadata = useSyncExternalStore(
      manager.subscribe,
      manager.getMetadata,
    );

    const stableMetadataResult = useMemo<UseHtmlMetadataResult>(() => {
      return {
        metadata,
        clearMetadataByKey: manager.clearMetadataByKey,
      };
    }, [metadata]);

    return stableMetadataResult;
  };
}

const defaultManager = new MetadataManager();
export const useHtmlMetadata = makeUseHtmlMetadata(defaultManager);
