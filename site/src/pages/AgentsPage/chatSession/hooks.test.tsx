import { renderHook } from "@testing-library/react";
import { act, type PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatSessionManager } from "./ChatSessionManager";
import { ChatSessionsProvider } from "./ChatSessionsProvider";
import {
	useChatSession,
	useChatSessionSelector,
	useChatSessionsManager,
} from "./hooks";
import type { ChatSessionManagerRuntimeDeps } from "./types";

type ChatErrorCallbacks = {
	setChatErrorReason: ChatSessionManagerRuntimeDeps["setChatErrorReason"];
	clearChatErrorReason: ChatSessionManagerRuntimeDeps["clearChatErrorReason"];
};

const createWrapperHarness = () => {
	const queryClient = createTestQueryClient();
	let callbacks: ChatErrorCallbacks = {
		setChatErrorReason: vi.fn(),
		clearChatErrorReason: vi.fn(),
	};

	const Wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>
			<ChatSessionsProvider
				setChatErrorReason={callbacks.setChatErrorReason}
				clearChatErrorReason={callbacks.clearChatErrorReason}
			>
				{children}
			</ChatSessionsProvider>
		</QueryClientProvider>
	);

	return {
		Wrapper,
		setCallbacks: (nextCallbacks: ChatErrorCallbacks) => {
			callbacks = nextCallbacks;
		},
	};
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("chat session provider hooks", () => {
	it("throws outside the provider", () => {
		vi.spyOn(console, "error").mockImplementation(() => {});

		expect(() => {
			renderHook(() => useChatSessionsManager());
		}).toThrow(
			"useChatSessionsManager must be used inside <ChatSessionsProvider>",
		);
	});

	it("creates one manager per provider mount", () => {
		const harness = createWrapperHarness();
		const { result, rerender } = renderHook(() => useChatSessionsManager(), {
			wrapper: harness.Wrapper,
		});
		const manager = result.current;

		harness.setCallbacks({
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		});
		rerender();

		expect(result.current).toBe(manager);
	});

	it("updates runtime dependencies without recreating the manager", () => {
		const updateRuntimeDepsSpy = vi.spyOn(
			ChatSessionManager.prototype,
			"updateRuntimeDeps",
		);
		const harness = createWrapperHarness();
		const { result, rerender } = renderHook(() => useChatSessionsManager(), {
			wrapper: harness.Wrapper,
		});
		const manager = result.current;
		const initialCallCount = updateRuntimeDepsSpy.mock.calls.length;
		const nextCallbacks = {
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		};

		harness.setCallbacks(nextCallbacks);
		rerender();

		expect(result.current).toBe(manager);
		expect(updateRuntimeDepsSpy.mock.calls.length).toBeGreaterThan(
			initialCallCount,
		);
		expect(updateRuntimeDepsSpy).toHaveBeenLastCalledWith(
			expect.objectContaining(nextCallbacks),
		);
	});

	it("disposes the manager on unmount", () => {
		const disposeSpy = vi.spyOn(ChatSessionManager.prototype, "dispose");
		const harness = createWrapperHarness();
		const { unmount } = renderHook(() => useChatSessionsManager(), {
			wrapper: harness.Wrapper,
		});

		unmount();

		expect(disposeSpy).toHaveBeenCalledTimes(1);
	});

	it("returns the manager-owned session for a chat", () => {
		const harness = createWrapperHarness();
		const { result } = renderHook(
			() => {
				const manager = useChatSessionsManager();
				const session = useChatSession("chat-1");
				return { manager, session };
			},
			{ wrapper: harness.Wrapper },
		);

		expect(result.current.session).toBe(
			result.current.manager.getOrCreate("chat-1"),
		);
	});

	it("selects metadata changes without subscribing to store changes", () => {
		const harness = createWrapperHarness();
		let renderCount = 0;
		const { result } = renderHook(
			() => {
				renderCount += 1;
				const manager = useChatSessionsManager();
				const followMode = useChatSessionSelector(
					"chat-1",
					(snapshot) => snapshot.followMode,
				);
				return { manager, followMode };
			},
			{ wrapper: harness.Wrapper },
		);
		const session = result.current.manager.getOrCreate("chat-1");
		const initialRenderCount = renderCount;

		act(() => {
			session.setFollowMode(false);
		});

		expect(result.current.followMode).toBe(false);
		expect(renderCount).toBeGreaterThan(initialRenderCount);
		const metadataRenderCount = renderCount;

		act(() => {
			session.store.setChatStatus("running");
		});

		expect(renderCount).toBe(metadataRenderCount);
	});
});
