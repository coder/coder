import type { WebpushMessage } from "#/api/typesGenerated";

// We need to mock the ServiceWorkerGlobalScope before importing the
// module, since serviceWorker.ts registers event listeners at the
// top level during module evaluation.

const mockShowNotification = vi.fn(() => Promise.resolve());
const mockSubscribe = vi.fn();
const mockRegistration = {
	showNotification: mockShowNotification,
	pushManager: { subscribe: mockSubscribe },
};
const mockMatchAll =
	vi.fn<
		() => Promise<
			Array<{ visibilityState: string; url: string; focused: boolean }>
		>
	>();
const mockClients = {
	matchAll: mockMatchAll,
	claim: vi.fn(() => Promise.resolve()),
	openWindow: vi.fn(() => Promise.resolve(null)),
};

// Collect handlers registered via addEventListener so we can
// invoke them directly in tests.
const handlers: Record<string, (event: unknown) => void> = {};
const mockSelf = {
	addEventListener: vi.fn(
		(event: string, handler: (event: unknown) => void) => {
			handlers[event] = handler;
		},
	),
	skipWaiting: vi.fn(),
	clients: mockClients,
	registration: mockRegistration,
};

// Assign our mock to the global `self` that serviceWorker.ts
// references via `declare const self`.
Object.assign(globalThis, { self: mockSelf });

// Helper to build a minimal PushEvent-like object that carries
// a JSON payload and exposes waitUntil so we can await the
// handler's async work.
function makePushEvent(payload: WebpushMessage) {
	let waitUntilPromise: Promise<unknown> = Promise.resolve();
	return {
		data: {
			json: () => payload,
		},
		waitUntil: (p: Promise<unknown>) => {
			waitUntilPromise = p;
		},
		// Expose the promise so tests can await it.
		get _waitUntilPromise() {
			return waitUntilPromise;
		},
	};
}

const testPayload: WebpushMessage = {
	title: "Test Notification",
	body: "Something happened",
	icon: "/icon.png",
	actions: [],
	data: { url: "/agents/abc" },
};

// Import the service worker module. This executes the top-level
// addEventListener calls which populate our `handlers` map.
beforeAll(async () => {
	await import("./serviceWorker");
});

beforeEach(() => {
	vi.clearAllMocks();
});

describe("serviceWorker push handler", () => {
	it("shows notification when no visible agents window exists", async () => {
		mockMatchAll.mockResolvedValue([]);

		const event = makePushEvent(testPayload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith(testPayload.title, {
			body: testPayload.body,
			icon: testPayload.icon,
			data: testPayload.data,
			tag: undefined,
		});
	});

	it("suppresses notification when viewing the specific chat", async () => {
		mockMatchAll.mockResolvedValue([
			{
				visibilityState: "visible",
				url: "https://example.com/agents/abc",
				focused: true,
			},
		]);

		const event = makePushEvent(testPayload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).not.toHaveBeenCalled();
	});

	it("shows notification when viewing a different chat", async () => {
		mockMatchAll.mockResolvedValue([
			{
				visibilityState: "visible",
				url: "https://example.com/agents/other-chat-id",
				focused: true,
			},
		]);

		const event = makePushEvent(testPayload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith(testPayload.title, {
			body: testPayload.body,
			icon: testPayload.icon,
			data: testPayload.data,
			tag: undefined,
		});
	});

	it("shows notification when payload has no data url", async () => {
		mockMatchAll.mockResolvedValue([
			{
				visibilityState: "visible",
				url: "https://example.com/agents/abc",
				focused: true,
			},
		]);

		const payload: WebpushMessage = {
			title: "No Data",
			body: "test",
			icon: "/icon.png",
			actions: [],
		};
		const event = makePushEvent(payload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith("No Data", {
			body: "test",
			icon: "/icon.png",
			data: undefined,
			tag: undefined,
		});
	});

	it("shows notification when specific chat page exists but is hidden", async () => {
		mockMatchAll.mockResolvedValue([
			{
				visibilityState: "hidden",
				url: "https://example.com/agents/abc",
				focused: false,
			},
		]);

		const event = makePushEvent(testPayload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith(testPayload.title, {
			body: testPayload.body,
			icon: testPayload.icon,
			data: testPayload.data,
			tag: undefined,
		});
	});

	it("shows notification when viewing the specific chat but browser is not focused", async () => {
		mockMatchAll.mockResolvedValue([
			{
				visibilityState: "visible",
				url: "https://example.com/agents/abc",
				focused: false,
			},
		]);

		const event = makePushEvent(testPayload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith(testPayload.title, {
			body: testPayload.body,
			icon: testPayload.icon,
			data: testPayload.data,
			tag: undefined,
		});
	});

	it("shows notification when visible window is not on agents page", async () => {
		mockMatchAll.mockResolvedValue([
			{
				visibilityState: "visible",
				url: "https://example.com/settings",
				focused: true,
			},
		]);

		const event = makePushEvent(testPayload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith(testPayload.title, {
			body: testPayload.body,
			icon: testPayload.icon,
			data: testPayload.data,
			tag: undefined,
		});
	});

	it("does not show notification when push event has no data", () => {
		const event = { data: null };
		// Should return early without calling waitUntil, so no
		// error and no notification.
		handlers.push(event);
		expect(mockShowNotification).not.toHaveBeenCalled();
	});

	it("uses default icon when payload icon is empty", async () => {
		mockMatchAll.mockResolvedValue([]);

		const payload: WebpushMessage = {
			title: "No Icon",
			body: "",
			icon: "",
			actions: [],
		};
		const event = makePushEvent(payload);
		handlers.push(event);
		await event._waitUntilPromise;

		expect(mockShowNotification).toHaveBeenCalledWith("No Icon", {
			body: "",
			icon: "/favicon.ico",
			data: undefined,
			tag: undefined,
		});
	});
});

// Helper to build a minimal NotificationEvent-like object for
// the notificationclick handler.
function makeNotificationClickEvent(url?: string) {
	let waitUntilPromise: Promise<unknown> = Promise.resolve();
	return {
		notification: {
			close: vi.fn(),
			data: url !== undefined ? { url } : undefined,
		},
		waitUntil: (p: Promise<unknown>) => {
			waitUntilPromise = p;
		},
		// Expose the promise so tests can await it.
		get _waitUntilPromise() {
			return waitUntilPromise;
		},
	};
}

describe("serviceWorker notificationclick handler", () => {
	it("focuses existing client without navigating when already on the target URL", async () => {
		const mockClient = {
			url: "https://example.com/agents/abc",
			visibilityState: "visible",
			focused: true,
			focus: vi.fn(() => Promise.resolve()),
			navigate: vi.fn(() => Promise.resolve()),
		};
		mockMatchAll.mockResolvedValue([mockClient]);

		const event = makeNotificationClickEvent("/agents/abc");
		handlers.notificationclick(event);
		await event._waitUntilPromise;

		expect(event.notification.close).toHaveBeenCalled();
		expect(mockClient.navigate).not.toHaveBeenCalled();
		expect(mockClient.focus).toHaveBeenCalled();
	});

	it("navigates and focuses existing client when on a different agents page", async () => {
		const mockClient = {
			url: "https://example.com/agents/other",
			visibilityState: "visible",
			focused: true,
			focus: vi.fn(() => Promise.resolve()),
			navigate: vi.fn(() => Promise.resolve()),
		};
		mockMatchAll.mockResolvedValue([mockClient]);

		const event = makeNotificationClickEvent("/agents/abc");
		handlers.notificationclick(event);
		await event._waitUntilPromise;

		expect(event.notification.close).toHaveBeenCalled();
		expect(mockClient.navigate).toHaveBeenCalledWith("/agents/abc");
		expect(mockClient.focus).toHaveBeenCalled();
	});

	it("opens new window when no matching client exists", async () => {
		mockMatchAll.mockResolvedValue([]);

		const event = makeNotificationClickEvent("/agents/abc");
		handlers.notificationclick(event);
		await event._waitUntilPromise;

		expect(event.notification.close).toHaveBeenCalled();
		expect(mockClients.openWindow).toHaveBeenCalledWith("/agents/abc");
	});

	it("defaults to /agents when notification has no data url", async () => {
		mockMatchAll.mockResolvedValue([]);

		const event = makeNotificationClickEvent();
		handlers.notificationclick(event);
		await event._waitUntilPromise;

		expect(event.notification.close).toHaveBeenCalled();
		expect(mockClients.openWindow).toHaveBeenCalledWith("/agents");
	});
});

// Helper to build a minimal PushSubscriptionChangeEvent-like object.
// Mirrors the spec shape consumed by the service worker handler.
function makePushSubscriptionChangeEvent(
	oldSubscription: {
		endpoint: string;
		options?: { applicationServerKey?: ArrayBuffer | Uint8Array | string };
	} | null,
	newSubscription: {
		endpoint: string;
		options?: { applicationServerKey?: ArrayBuffer | Uint8Array | string };
		toJSON: () => unknown;
	} | null,
) {
	let waitUntilPromise: Promise<unknown> = Promise.resolve();
	return {
		oldSubscription,
		newSubscription,
		waitUntil: (p: Promise<unknown>) => {
			waitUntilPromise = p;
		},
		get _waitUntilPromise() {
			return waitUntilPromise;
		},
	};
}

describe("serviceWorker pushsubscriptionchange handler", () => {
	const originalFetch = globalThis.fetch;
	let mockFetch: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		mockFetch = vi.fn(() =>
			Promise.resolve({ ok: true, status: 204 } as Response),
		);
		globalThis.fetch = mockFetch as unknown as typeof fetch;
	});

	afterEach(() => {
		globalThis.fetch = originalFetch;
	});

	it("re-subscribes and POSTs the new subscription when the browser supplies one", async () => {
		const applicationServerKey = new Uint8Array([1, 2, 3]).buffer;
		const newSubscription = {
			endpoint: "https://push.example.com/new",
			options: { applicationServerKey },
			toJSON: () => ({
				endpoint: "https://push.example.com/new",
				keys: { auth: "new-auth", p256dh: "new-p256dh" },
			}),
		};
		const oldSubscription = {
			endpoint: "https://push.example.com/old",
			options: { applicationServerKey },
		};

		const event = makePushSubscriptionChangeEvent(
			oldSubscription,
			newSubscription,
		);
		handlers.pushsubscriptionchange(event);
		await event._waitUntilPromise;

		// The browser already supplied newSubscription, so we must
		// not call subscribe again.
		expect(mockSubscribe).not.toHaveBeenCalled();

		// Should have POSTed the new subscription and remove
		// the old endpoint via DELETE.
		const calls = mockFetch.mock.calls;
		expect(calls).toHaveLength(2);

		const [postUrl, postInit] = calls[0];
		expect(postUrl).toBe("/api/v2/users/me/webpush/subscription");
		expect(postInit?.method).toBe("POST");
		const postBody = JSON.parse(postInit?.body as string);
		expect(postBody).toEqual({
			endpoint: "https://push.example.com/new",
			auth_key: "new-auth",
			p256dh_key: "new-p256dh",
		});

		const [deleteUrl, deleteInit] = calls[1];
		expect(deleteUrl).toBe("/api/v2/users/me/webpush/subscription");
		expect(deleteInit?.method).toBe("DELETE");
		const deleteBody = JSON.parse(deleteInit?.body as string);
		expect(deleteBody).toEqual({
			endpoint: "https://push.example.com/old",
		});
	});

	it("re-subscribes via pushManager when no newSubscription is provided", async () => {
		const applicationServerKey = new Uint8Array([4, 5, 6]).buffer;
		const oldSubscription = {
			endpoint: "https://push.example.com/old",
			options: { applicationServerKey },
		};
		const freshSubscription = {
			endpoint: "https://push.example.com/fresh",
			toJSON: () => ({
				endpoint: "https://push.example.com/fresh",
				keys: { auth: "fresh-auth", p256dh: "fresh-p256dh" },
			}),
		};
		mockSubscribe.mockResolvedValue(freshSubscription);

		const event = makePushSubscriptionChangeEvent(oldSubscription, null);
		handlers.pushsubscriptionchange(event);
		await event._waitUntilPromise;

		expect(mockSubscribe).toHaveBeenCalledWith({
			userVisibleOnly: true,
			applicationServerKey,
		});

		const calls = mockFetch.mock.calls;
		expect(calls).toHaveLength(2);
		const postBody = JSON.parse(calls[0][1]?.body as string);
		expect(postBody).toEqual({
			endpoint: "https://push.example.com/fresh",
			auth_key: "fresh-auth",
			p256dh_key: "fresh-p256dh",
		});
	});

	it("does nothing when there is no application server key to recover", async () => {
		const event = makePushSubscriptionChangeEvent(null, null);
		handlers.pushsubscriptionchange(event);
		await event._waitUntilPromise;

		expect(mockSubscribe).not.toHaveBeenCalled();
		expect(mockFetch).not.toHaveBeenCalled();
	});
});
