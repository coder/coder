/**
 * @file The test setup for this file is a little funky because of how React
 * Testing Library works.
 *
 * When you call user.setup to make a new user session, it will make a mock
 * clipboard instance that will always succeed. It also can't be removed after
 * it's been added, and it will persist across test cases. This actually makes
 * testing useClipboard properly impossible because any call to user.setup
 * immediately pollutes the tests with false negatives. Even if something should
 * fail, it won't.
 */

import { renderHook, screen } from "@testing-library/react";
import { GlobalSnackbar } from "components/GlobalSnackbar/GlobalSnackbar";
import { ThemeOverride } from "contexts/ThemeProvider";
import { act } from "react";
import themes, { DEFAULT_THEME } from "theme";
import {
	COPY_FAILED_MESSAGE,
	HTTP_FALLBACK_DATA_ID,
	type UseClipboardInput,
	type UseClipboardResult,
	useClipboard,
} from "./useClipboard";

// Need to mock console.error because we deliberately need to trigger errors in
// the code to assert that it can recover from them, but we also don't want them
// logged if they're expected
const originalConsoleError = console.error;

type SetupMockClipboardResult = Readonly<{
	mockClipboard: Clipboard;
	mockExecCommand: typeof global.document.execCommand;
	getClipboardText: () => string;
	setSimulateFailure: (shouldFail: boolean) => void;
	resetMockClipboardState: () => void;
}>;

function setupMockClipboard(isSecure: boolean): SetupMockClipboardResult {
	let mockClipboardText = "";
	let shouldSimulateFailure = false;

	const mockClipboard: Clipboard = {
		readText: async () => {
			if (!isSecure) {
				throw new Error(
					"Not allowed to access clipboard outside of secure contexts",
				);
			}

			if (shouldSimulateFailure) {
				throw new Error("Failed to read from clipboard");
			}

			return mockClipboardText;
		},

		writeText: async (newText) => {
			if (!isSecure) {
				throw new Error(
					"Not allowed to access clipboard outside of secure contexts",
				);
			}

			if (shouldSimulateFailure) {
				throw new Error("Failed to write to clipboard");
			}

			mockClipboardText = newText;
		},

		// Don't need these other methods for any of the tests; read and write are
		// both synchronous and slower than the promise-based methods, so ideally
		// we won't ever need to call them in the hook logic
		addEventListener: jest.fn(),
		removeEventListener: jest.fn(),
		dispatchEvent: jest.fn(),
		read: jest.fn(),
		write: jest.fn(),
	};

	return {
		mockClipboard,
		getClipboardText: () => mockClipboardText,
		setSimulateFailure: (newShouldFailValue) => {
			shouldSimulateFailure = newShouldFailValue;
		},
		resetMockClipboardState: () => {
			shouldSimulateFailure = false;
			mockClipboardText = "";
		},
		mockExecCommand: (commandId) => {
			if (commandId !== "copy") {
				return false;
			}

			if (shouldSimulateFailure) {
				throw new Error("Failed to execute command 'copy'");
			}

			const dummyInput = document.querySelector(
				`input[data-testid=${HTTP_FALLBACK_DATA_ID}]`,
			);

			const inputIsFocused =
				dummyInput instanceof HTMLInputElement &&
				document.activeElement === dummyInput;

			let copySuccessful = false;
			if (inputIsFocused) {
				mockClipboardText = dummyInput.value;
				copySuccessful = true;
			}

			return copySuccessful;
		},
	};
}

function renderUseClipboard(inputs?: UseClipboardInput) {
	return renderHook<UseClipboardResult, UseClipboardInput>(
		(props) => useClipboard(props),
		{
			initialProps: inputs,
			wrapper: ({ children }) => (
				// Need ThemeProvider because GlobalSnackbar uses theme
				<ThemeOverride theme={themes[DEFAULT_THEME]}>
					{children}
					<GlobalSnackbar />
				</ThemeOverride>
			),
		},
	);
}

type RenderResult = ReturnType<typeof renderUseClipboard>["result"];

// execCommand is the workaround for copying text to the clipboard on HTTP-only
// connections
const originalExecCommand = global.document.execCommand;
const originalNavigator = window.navigator;

// Not a big fan of describe.each most of the time, but since we need to test
// the exact same test cases against different inputs, and we want them to run
// as sequentially as possible to minimize flakes, they make sense here
const secureContextValues: readonly boolean[] = [true, false];
describe.each(secureContextValues)("useClipboard - secure: %j", (isSecure) => {
	const {
		mockClipboard,
		mockExecCommand,
		getClipboardText,
		setSimulateFailure,
		resetMockClipboardState,
	} = setupMockClipboard(isSecure);

	beforeEach(() => {
		jest.useFakeTimers();

		// Can't use jest.spyOn here because there's no guarantee that the mock
		// browser environment actually implements execCommand. Trying to spy on an
		// undefined value will throw an error
		global.document.execCommand = mockExecCommand;

		jest.spyOn(window, "navigator", "get").mockImplementation(() => ({
			...originalNavigator,
			clipboard: mockClipboard,
		}));

		jest.spyOn(console, "error").mockImplementation((errorValue, ...rest) => {
			const canIgnore =
				errorValue instanceof Error &&
				errorValue.message === COPY_FAILED_MESSAGE;

			if (!canIgnore) {
				originalConsoleError(errorValue, ...rest);
			}
		});
	});

	afterEach(() => {
		jest.runAllTimers();
		jest.useRealTimers();
		jest.resetAllMocks();
		global.document.execCommand = originalExecCommand;

		// Still have to reset the mock clipboard state because the same mock values
		// are reused for each test case in a given describe.each iteration
		resetMockClipboardState();
	});

	const assertClipboardUpdateLifecycle = async (
		result: RenderResult,
		textToCopy: string,
	): Promise<void> => {
		await act(() => result.current.copyToClipboard(textToCopy));
		expect(result.current.showCopiedSuccess).toBe(true);

		// Because of timing trickery, any timeouts for flipping the copy status
		// back to false will usually trigger before any test cases calling this
		// assert function can complete. This will never be an issue in the real
		// world, but it will kick up 'act' warnings in the console, which makes
		// tests more annoying. Getting around that by waiting for all timeouts to
		// wrap up, but note that the value of showCopiedSuccess will become false
		// after runAllTimersAsync finishes
		await act(() => jest.runAllTimersAsync());

		const clipboardText = getClipboardText();
		expect(clipboardText).toEqual(textToCopy);
	};

	it("Copies the current text to the user's clipboard", async () => {
		const textToCopy = "dogs";
		const { result } = renderUseClipboard();
		await assertClipboardUpdateLifecycle(result, textToCopy);
	});

	it("Should indicate to components not to show successful copy after a set period of time", async () => {
		const textToCopy = "cats";
		const { result } = renderUseClipboard();
		await assertClipboardUpdateLifecycle(result, textToCopy);
		expect(result.current.showCopiedSuccess).toBe(false);
	});

	it("Should notify the user of an error using the provided callback", async () => {
		const textToCopy = "birds";
		const onError = jest.fn();
		const { result } = renderUseClipboard({ onError });

		setSimulateFailure(true);
		await act(() => result.current.copyToClipboard(textToCopy));
		expect(onError).toBeCalled();
	});

	it("Should dispatch a new toast message to the global snackbar when errors happen while no error callback is provided to the hook", async () => {
		const textToCopy = "crow";
		const { result } = renderUseClipboard();

		/**
		 * @todo Look into why deferring error-based state updates to the global
		 * snackbar still kicks up act warnings, even after wrapping copyToClipboard
		 * in act. copyToClipboard should be the main source of the state
		 * transitions, but it looks like extra state changes are still getting
		 * flushed through the GlobalSnackbar component afterwards
		 */
		setSimulateFailure(true);
		await act(() => result.current.copyToClipboard(textToCopy));

		const errorMessageNode = screen.queryByText(COPY_FAILED_MESSAGE);
		expect(errorMessageNode).not.toBeNull();
	});

	it("Should expose the error as a value when a copy fails", async () => {
		// Using empty onError callback to silence any possible act warnings from
		// Snackbar state transitions that you might get if the hook uses the
		// default
		const textToCopy = "hamster";
		const { result } = renderUseClipboard({ onError: jest.fn() });

		setSimulateFailure(true);
		await act(() => result.current.copyToClipboard(textToCopy));

		expect(result.current.error).toBeInstanceOf(Error);
	});

	it("Clears out existing errors if a new copy operation succeeds", async () => {
		const text = "dummy-text";
		const { result } = renderUseClipboard();
		setSimulateFailure(true);

		await act(() => result.current.copyToClipboard(text));
		expect(result.current.error).toBeInstanceOf(Error);

		setSimulateFailure(false);
		await assertClipboardUpdateLifecycle(result, text);
		expect(result.current.error).toBeUndefined();
	});

	// This test case is really important to ensure that it's easy to plop this
	// inside of useEffect calls without having to think about dependencies too
	// much
	it("Ensures that the copyToClipboard function always maintains a stable reference across all re-renders", async () => {
		const initialOnError = jest.fn();
		const { result, rerender } = renderUseClipboard({
			onError: initialOnError,
			clearErrorOnSuccess: true,
		});
		const initialCopy = result.current.copyToClipboard;

		// Re-render arbitrarily with no clipboard state transitions to make
		// sure that a parent re-rendering doesn't break anything
		rerender({ onError: initialOnError });
		expect(result.current.copyToClipboard).toBe(initialCopy);

		// Re-render with new onError prop and then swap back to simplify
		// testing
		rerender({ onError: jest.fn() });
		expect(result.current.copyToClipboard).toBe(initialCopy);
		rerender({ onError: initialOnError });

		// Re-render with a new clear value then swap back to simplify testing
		rerender({ onError: initialOnError, clearErrorOnSuccess: false });
		expect(result.current.copyToClipboard).toBe(initialCopy);
		rerender({ onError: initialOnError, clearErrorOnSuccess: true });

		// Trigger a failed clipboard interaction
		setSimulateFailure(true);
		await act(() => result.current.copyToClipboard("dummy-text-2"));
		expect(result.current.copyToClipboard).toBe(initialCopy);

		/**
		 * Trigger a successful clipboard interaction
		 *
		 * @todo For some reason, using the assertClipboardUpdateLifecycle
		 * helper triggers Jest errors with it thinking that values are being
		 * accessed after teardown, even though the problem doesn't exist for
		 * any other test case.
		 *
		 * It's not a huge deal, because we only need to inspect React after the
		 * interaction, instead of the full DOM, but for correctness, it would
		 * be nice if we could get this issue figured out.
		 */
		setSimulateFailure(false);
		await act(() => result.current.copyToClipboard("dummy-text-2"));
		expect(result.current.copyToClipboard).toBe(initialCopy);
	});

	it("Always uses the most up-to-date onError prop", async () => {
		const initialOnError = jest.fn();
		const { result, rerender } = renderUseClipboard({
			onError: initialOnError,
		});
		setSimulateFailure(true);

		const secondOnError = jest.fn();
		rerender({ onError: secondOnError });
		await act(() => result.current.copyToClipboard("dummy-text"));

		expect(initialOnError).not.toHaveBeenCalled();
		expect(secondOnError).toHaveBeenCalledTimes(1);
		expect(secondOnError).toHaveBeenCalledWith(
			"Failed to copy text to clipboard",
		);
	});
});
