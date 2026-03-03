import { renderHook, act } from "@testing-library/react";
import { useWebpushNotifications } from "./useWebpushNotifications";

// Mock react-query
jest.mock("react-query", () => ({
	useQuery: (queryOptions: { queryKey: string[] }) => {
		if (queryOptions.queryKey?.includes("buildInfo")) {
			return {
				data: { webpush_public_key: "test-vapid-key" },
				isLoading: false,
			};
		}
		if (queryOptions.queryKey?.includes("experiments")) {
			return { data: ["web-push"], isLoading: false };
		}
		return { data: undefined, isLoading: false };
	},
}));

// Mock useEmbeddedMetadata
jest.mock("hooks/useEmbeddedMetadata", () => ({
	useEmbeddedMetadata: () => ({
		metadata: {
			"build-info": undefined,
			experiments: undefined,
		},
	}),
}));

// Mock the API module
jest.mock("api/api", () => ({
	API: {
		createWebPushSubscription: jest.fn().mockResolvedValue(undefined),
	},
}));

// Mock query key factories to return objects with queryKey
jest.mock("api/queries/buildInfo", () => ({
	buildInfo: () => ({ queryKey: ["buildInfo"] }),
}));
jest.mock("api/queries/experiments", () => ({
	experiments: () => ({ queryKey: ["experiments"] }),
}));

// Store the original globals so we can restore them.
const originalNotification = globalThis.Notification;
const originalServiceWorker = navigator.serviceWorker;

afterEach(() => {
	jest.restoreAllMocks();
	Object.defineProperty(globalThis, "Notification", {
		value: originalNotification,
		writable: true,
		configurable: true,
	});
	Object.defineProperty(navigator, "serviceWorker", {
		value: originalServiceWorker,
		writable: true,
		configurable: true,
	});
});

function setupBrowserMocks(
	permissionResult: NotificationPermission = "granted",
) {
	const mockSubscription = {
		endpoint: "https://push.example.com/test",
		toJSON: () => ({
			endpoint: "https://push.example.com/test",
			keys: { auth: "auth-key", p256dh: "p256dh-key" },
		}),
		unsubscribe: jest.fn().mockResolvedValue(true),
	};

	const mockPushManager = {
		subscribe: jest.fn().mockResolvedValue(mockSubscription),
		getSubscription: jest.fn().mockResolvedValue(null),
	};

	const mockRegistration = { pushManager: mockPushManager };

	// Mock Notification with requestPermission
	const MockNotification = jest.fn() as unknown as typeof Notification;
	Object.defineProperty(MockNotification, "requestPermission", {
		value: jest.fn().mockResolvedValue(permissionResult),
		writable: true,
	});
	Object.defineProperty(MockNotification, "permission", {
		value: permissionResult === "granted" ? "default" : permissionResult,
		writable: true,
	});
	Object.defineProperty(globalThis, "Notification", {
		value: MockNotification,
		writable: true,
		configurable: true,
	});

	// Mock serviceWorker
	Object.defineProperty(navigator, "serviceWorker", {
		value: {
			ready: Promise.resolve(mockRegistration),
			register: jest.fn(),
		},
		writable: true,
		configurable: true,
	});

	return { mockPushManager, MockNotification };
}

describe("useWebpushNotifications", () => {
	it("requests notification permission before subscribing", async () => {
		const { MockNotification, mockPushManager } = setupBrowserMocks("granted");

		const { result } = renderHook(() => useWebpushNotifications());

		// Wait for initial state to settle.
		await act(async () => {});

		await act(async () => {
			await result.current.subscribe();
		});

		// Permission should be requested before pushManager.subscribe.
		expect(MockNotification.requestPermission).toHaveBeenCalledTimes(1);
		expect(mockPushManager.subscribe).toHaveBeenCalledTimes(1);
	});

	it("throws a clear error when permission is denied", async () => {
		setupBrowserMocks("denied");

		const { result } = renderHook(() => useWebpushNotifications());

		await act(async () => {});

		await expect(
			act(async () => {
				await result.current.subscribe();
			}),
		).rejects.toThrow(/blocked by your browser/i);
	});

	it("does not call pushManager.subscribe when permission is denied", async () => {
		const { mockPushManager } = setupBrowserMocks("denied");

		const { result } = renderHook(() => useWebpushNotifications());

		await act(async () => {});

		try {
			await act(async () => {
				await result.current.subscribe();
			});
		} catch {
			// Expected to throw.
		}

		expect(mockPushManager.subscribe).not.toHaveBeenCalled();
	});
});
