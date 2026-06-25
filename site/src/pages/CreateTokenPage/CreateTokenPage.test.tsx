import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { HTTP_FALLBACK_DATA_ID } from "#/hooks/useClipboard";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import CreateTokenPage from "./CreateTokenPage";
import { NANO_HOUR } from "./utils";

describe("TokenPage", () => {
	const originalExecCommand = document.execCommand;
	const originalNavigator = navigator;

	afterEach(() => {
		document.execCommand = originalExecCommand;
		vi.restoreAllMocks();
	});

	const createToken = async () => {
		vi.spyOn(API, "getTokenConfig").mockResolvedValue({
			max_token_lifetime: 90 * 24 * NANO_HOUR,
		});
		vi.spyOn(API, "createToken").mockResolvedValueOnce({
			key: "abcd",
		});

		const { container } = renderWithAuth(<CreateTokenPage />, {
			route: "/settings/tokens/new",
			path: "/settings/tokens/new",
		});
		await waitForLoaderToBeRemoved();

		const form = container.querySelector("form") as HTMLFormElement;
		await userEvent.type(screen.getByLabelText(/Name/), "my-token");
		await userEvent.click(
			within(form).getByRole("button", { name: /create token/i }),
		);
	};

	it("shows the success modal", async () => {
		// When
		await createToken();

		// Then
		expect(screen.getByText("abcd")).toBeInTheDocument();
	});

	it("selects only the created token from the success modal", async () => {
		await createToken();

		const tokenContainer = screen.getByText("abcd").closest("div");
		expect(tokenContainer).toBeInTheDocument();

		const selectedRange = document.createRange();
		selectedRange.selectNodeContents(tokenContainer as HTMLElement);

		const selection = window.getSelection();
		selection?.removeAllRanges();
		selection?.addRange(selectedRange);

		expect(selection?.toString().trim()).toBe("abcd");
		selection?.removeAllRanges();
	});

	it("copies the created token from the success modal when clipboard fallback is used", async () => {
		const mockClipboard: Clipboard = {
			...originalNavigator.clipboard,
			writeText: vi.fn().mockRejectedValue(new Error("Clipboard unavailable")),
		};
		vi.spyOn(window, "navigator", "get").mockImplementation(() => ({
			...originalNavigator,
			clipboard: mockClipboard,
		}));

		let copiedText = "";
		document.execCommand = vi.fn((commandId) => {
			const dummyInput = document.querySelector(
				`input[data-testid=${HTTP_FALLBACK_DATA_ID}]`,
			);
			const inputCanReceiveDialogFocus =
				commandId === "copy" &&
				dummyInput instanceof HTMLInputElement &&
				dummyInput.closest('[role="dialog"]') !== null &&
				document.activeElement === dummyInput;

			if (!inputCanReceiveDialogFocus) {
				return false;
			}

			copiedText = dummyInput.value;
			return true;
		});

		await createToken();
		await userEvent.click(
			await screen.findByRole("button", { name: "Copy code" }),
		);

		await waitFor(() => {
			expect(copiedText).toBe("abcd");
		});
	});
});
