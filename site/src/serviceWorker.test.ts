import type { WebpushMessage } from "api/typesGenerated";

// We need to mock the ServiceWorkerGlobalScope before importing the
// module, since serviceWorker.ts registers event listeners at the
// top level during module evaluation.

const mockShowNotification = vi.fn(() => Promise.resolve());
const mockRegistration = { showNotification: mockShowNotification };
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
