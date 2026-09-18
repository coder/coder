import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, type ReactNode } from "react";
import { toast } from "sonner";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { createMockFile } from "#/testHelpers/files";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ organizations: [] }),
}));

const modelOptions = [
	{
		id: "model-config-1",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const renderInput = (children: ReactNode) => {
	return render(<AppProviders>{children}</AppProviders>);
};

const defaultProps = {
	onSend: vi.fn(),
	isDisabled: false,
	isLoading: false,
	selectedModel: modelOptions[0].id,
	onModelChange: vi.fn(),
	modelOptions,
	modelSelectorPlaceholder: "Select model",
	hasModelOptions: true,
	canConfigureAgentSetup: false,
} as const;

const mobileDropdownVariables = [
	"--mobile-dropdown-bottom",
	"--mobile-dropdown-left",
	"--mobile-dropdown-width",
	"--mobile-dropdown-above-composer-bottom",
	"--mobile-dropdown-above-composer-max-height",
];

// The jsdom matchMedia polyfill reports no query as matching, so tests run
// at a desktop viewport unless they stub the below-md query.
const stubViewportBelowMd = () => {
	const original = window.matchMedia;
	vi.spyOn(window, "matchMedia").mockImplementation((query: string) => ({
		...original(query),
		matches: query === belowMdViewportMediaQuery,
	}));
};

beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("AgentChatInput", () => {
	it("accepts drafts without sending while submission is disabled", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={onSend}
				inputRef={inputRef}
				isDisabled
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Draft while models load");
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("Draft while models load");
		});
		await user.keyboard("{Enter}");
		expect(onSend).not.toHaveBeenCalled();
	});

	it("attaches supported dropped files and reports unsupported ones", () => {
		const onAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const svg = createMockFile("diagram.svg", "image/svg+xml");
		const zip = createMockFile("archive.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [svg, zip] },
		});

		expect(onAttach).toHaveBeenCalledWith([svg]);
		expect(toastError).toHaveBeenCalledWith(
			"Unsupported file type: archive.zip",
		);
	});

	// Writing custom properties on <html> restyles the whole document, which
	// blocked focus and typing for seconds in long chats. The variables
	// only feed the below-md dropdown utilities, so wider viewports must not
	// touch the root element at all.
	it("does not write mobile dropdown variables on the root element at desktop widths", async () => {
		const user = userEvent.setup();
		const setProperty = vi.spyOn(document.documentElement.style, "setProperty");

		renderInput(<AgentChatInput {...defaultProps} />);
		const textbox = screen.getByRole("textbox", { name: "Chat message" });
		await user.click(textbox);
		await user.keyboard("when");

		const rootWrites = setProperty.mock.calls.filter(([name]) =>
			name.startsWith("--mobile-dropdown-"),
		);
		expect(rootWrites).toEqual([]);
		for (const name of mobileDropdownVariables) {
			expect(document.documentElement.style.getPropertyValue(name)).toBe("");
		}
	});

	it("positions mobile dropdowns from the composer below the md breakpoint without rewriting unchanged values", async () => {
		stubViewportBelowMd();
		const user = userEvent.setup();
		const setProperty = vi.spyOn(document.documentElement.style, "setProperty");

		const { unmount } = renderInput(<AgentChatInput {...defaultProps} />);
		for (const name of mobileDropdownVariables) {
			expect(document.documentElement.style.getPropertyValue(name)).toMatch(
				/^\d+px$/,
			);
		}
		const writesAfterMount = setProperty.mock.calls.length;

		// Focus and typing re-read the geometry; jsdom reports the same
		// rectangles, so the values are unchanged and must not be rewritten.
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("when");
		expect(setProperty.mock.calls.length).toBe(writesAfterMount);

		unmount();
		for (const name of mobileDropdownVariables) {
			expect(document.documentElement.style.getPropertyValue(name)).toBe("");
		}
	});
});
