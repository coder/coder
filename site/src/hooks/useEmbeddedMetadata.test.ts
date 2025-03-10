import { act, renderHook } from "@testing-library/react";
import type { Region, User } from "api/typesGenerated";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockEntitlements,
	MockExperiments,
	MockUser,
	MockUserAppearanceSettings,
} from "testHelpers/entities";
import {
	DEFAULT_METADATA_KEY,
	type MetadataKey,
	MetadataManager,
	type MetadataValue,
	type RuntimeHtmlMetadata,
	makeUseEmbeddedMetadata,
	useEmbeddedMetadata,
} from "./useEmbeddedMetadata";

// Make sure that no matter what happens in the tests, all metadata is
// eventually deleted
const allAppendedNodes = new Set<Set<Element>>();
afterAll(() => {
	for (const tracker of allAppendedNodes) {
		for (const node of tracker) {
			node.remove();
		}
	}
});

// Using empty array for now, because we don't have a separate mock regions
// value, but it's still good enough for the tests because it's truthy
const MockRegions: readonly Region[] = [];

const mockDataForTags = {
	appearance: MockAppearanceConfig,
	"build-info": MockBuildInfo,
	entitlements: MockEntitlements,
	experiments: MockExperiments,
	user: MockUser,
	userAppearance: MockUserAppearanceSettings,
	regions: MockRegions,
} as const satisfies Record<MetadataKey, MetadataValue>;

const emptyMetadata: RuntimeHtmlMetadata = {
	appearance: {
		available: false,
		value: undefined,
	},
	"build-info": {
		available: false,
		value: undefined,
	},
	entitlements: {
		available: false,
		value: undefined,
	},
	experiments: {
		available: false,
		value: undefined,
	},
	regions: {
		available: false,
		value: undefined,
	},
	user: {
		available: false,
		value: undefined,
	},
	userAppearance: {
		available: false,
		value: undefined,
	},
};

const populatedMetadata: RuntimeHtmlMetadata = {
	appearance: {
		available: true,
		value: MockAppearanceConfig,
	},
	"build-info": {
		available: true,
		value: MockBuildInfo,
	},
	entitlements: {
		available: true,
		value: MockEntitlements,
	},
	experiments: {
		available: true,
		value: MockExperiments,
	},
	regions: {
		available: true,
		value: MockRegions,
	},
	user: {
		available: true,
		value: MockUser,
	},
	userAppearance: {
		available: true,
		value: MockUserAppearanceSettings,
	},
};

function seedInitialMetadata(metadataKey: string): () => void {
	// Enforcing this to make sure that even if we start to adopt more concurrent
	// tests through Vitest (or similar), there's no risk of global state causing
	// weird, hard-to-test false positives/negatives with other tests
	if (metadataKey === DEFAULT_METADATA_KEY) {
		throw new Error(
			"Please ensure that the key you provide does not match the key used throughout the majority of the application",
		);
	}

	const trackedNodes = new Set<Element>();
	allAppendedNodes.add(trackedNodes);

	for (const metadataName in mockDataForTags) {
		// Serializing first because that's the part that can fail; want to fail
		// early if possible
		const value = mockDataForTags[metadataName as keyof typeof mockDataForTags];
		const serialized = JSON.stringify(value);

		const newNode = document.createElement("meta");
		newNode.setAttribute(metadataKey, metadataName);
		newNode.setAttribute("content", serialized);
		document.head.append(newNode);

		trackedNodes.add(newNode);
	}

	return () => {
		for (const node of trackedNodes) {
			node.remove();
		}
	};
}

function renderMetadataHook(metadataKey: string) {
	const manager = new MetadataManager(metadataKey);
	const hook = makeUseEmbeddedMetadata(manager);

	return {
		...renderHook(hook),
		manager,
	};
}

// Just to be on the safe side, probably want to make sure that each test case
// is set up with a unique key
describe(useEmbeddedMetadata.name, () => {
	it("Correctly detects when metadata is missing in the HTML page", () => {
		const key = "cat";

		// Deliberately avoid seeding any metadata
		const { result } = renderMetadataHook(key);
		expect(result.current.metadata).toEqual<RuntimeHtmlMetadata>(emptyMetadata);
	});

	it("Can detect when metadata exists in the HTML", () => {
		const key = "dog";

		const cleanupTags = seedInitialMetadata(key);
		const { result } = renderMetadataHook(key);
		expect(result.current.metadata).toEqual<RuntimeHtmlMetadata>(
			populatedMetadata,
		);

		cleanupTags();
	});

	it("Lets external systems (including React) subscribe to when metadata values are deleted", () => {
		const key = "bird";
		const tag1: MetadataKey = "user";
		const tag2: MetadataKey = "appearance";

		const cleanupTags = seedInitialMetadata(key);
		const { result: reactResult, manager } = renderMetadataHook(key);

		const nonReactSubscriber = jest.fn();
		manager.subscribe(nonReactSubscriber);

		const expectedUpdate1: RuntimeHtmlMetadata = {
			...populatedMetadata,
			[tag1]: {
				available: false,
				value: undefined,
			},
		};

		// Test that updates work when run directly through the metadata manager
		// itself
		act(() => manager.clearMetadataByKey(tag1));
		expect(reactResult.current.metadata).toEqual(expectedUpdate1);
		expect(nonReactSubscriber).toBeCalledWith(expectedUpdate1);

		nonReactSubscriber.mockClear();
		const expectedUpdate2: RuntimeHtmlMetadata = {
			...expectedUpdate1,
			[tag2]: {
				available: false,
				value: undefined,
			},
		};

		// Test that updates work when calling the convenience function exposed
		// through the React hooks
		act(() => reactResult.current.clearMetadataByKey(tag2));
		expect(reactResult.current.metadata).toEqual(expectedUpdate2);
		expect(nonReactSubscriber).toBeCalledWith(expectedUpdate2);

		cleanupTags();
	});

	it("Will delete the original metadata node when you try deleting a metadata state value", () => {
		const key = "aardvark";
		const tagToDelete: MetadataKey = "user";

		const cleanupTags = seedInitialMetadata(key);
		const { result } = renderMetadataHook(key);

		const query = `meta[${key}=${tagToDelete}]`;
		let userNode = document.querySelector(query);
		expect(userNode).not.toBeNull();

		act(() => result.current.clearMetadataByKey(tagToDelete));
		userNode = document.querySelector(query);
		expect(userNode).toBeNull();

		cleanupTags();
	});

	it("Will not notify subscribers if you try deleting a metadata value that does not exist (or has already been deleted)", () => {
		const key = "giraffe";
		const tagToDelete: MetadataKey = "entitlements";

		const cleanupTags = seedInitialMetadata(key);
		const { result } = renderMetadataHook(key);
		act(() => result.current.clearMetadataByKey(tagToDelete));

		const resultBeforeNoOp = result.current.metadata;
		act(() => result.current.clearMetadataByKey(tagToDelete));
		expect(result.current.metadata).toBe(resultBeforeNoOp);

		cleanupTags();
	});

	// Need to guarantee this, or else we could have a good number of bugs in the
	// React UI
	it("Always treats metadata as immutable values during all deletions", () => {
		const key = "hamster";
		const tagToDelete: MetadataKey = "user";

		const cleanupTags = seedInitialMetadata(key);
		const { result } = renderMetadataHook(key);

		const initialResult = result.current.metadata;
		act(() => result.current.clearMetadataByKey(tagToDelete));
		const newResult = result.current.metadata;
		expect(initialResult).not.toBe(newResult);

		// Mutate the initial result, and make sure the change doesn't propagate to
		// the updated result
		const mutableUser = initialResult.user as {
			available: boolean;
			value: User | undefined;
		};

		mutableUser.available = false;
		mutableUser.value = undefined;
		expect(mutableUser).toEqual(newResult.user);
		expect(mutableUser).not.toBe(newResult.user);

		cleanupTags();
	});
});
