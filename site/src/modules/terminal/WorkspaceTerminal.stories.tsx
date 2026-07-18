import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { withWebSocket } from "#/testHelpers/storybook";
import { touchCapableMediaQuery } from "#/utils/mobile";
import { WorkspaceTerminal } from "./WorkspaceTerminal";

const wrappedOutput = "Visible-terminal-output-".repeat(8);
const visibleOutput = `${wrappedOutput}\r\nSecond line`;

const setTouchCapability = (touchCapable: boolean) => {
	const originalMatchMedia = window.matchMedia;
	const originalMaxTouchPoints = Object.getOwnPropertyDescriptor(
		navigator,
		"maxTouchPoints",
	);

	window.matchMedia = (query: string): MediaQueryList => ({
		matches: query === touchCapableMediaQuery ? touchCapable : false,
		media: query,
		onchange: null,
		addEventListener: () => undefined,
		removeEventListener: () => undefined,
		addListener: () => undefined,
		removeListener: () => undefined,
		dispatchEvent: () => true,
	});
	Object.defineProperty(navigator, "maxTouchPoints", {
		configurable: true,
		value: touchCapable ? 1 : 0,
	});

	return () => {
		window.matchMedia = originalMatchMedia;
		if (originalMaxTouchPoints) {
			Object.defineProperty(
				navigator,
				"maxTouchPoints",
				originalMaxTouchPoints,
			);
		} else {
			Reflect.deleteProperty(navigator, "maxTouchPoints");
		}
	};
};

const meta = {
	title: "modules/terminal/WorkspaceTerminal",
	component: WorkspaceTerminal,
	args: {
		agentId: "agent-id",
		autoFocus: false,
		reconnectionToken: "00000000-0000-4000-8000-000000000000",
	},
	parameters: {
		layout: "centered",
		pixel: { exclude: true },
		webSocket: [],
	},
	decorators: [
		withWebSocket,
		(Story) => (
			<div style={{ width: 640, height: 480 }}>
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof WorkspaceTerminal>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TouchClipboardToolbar: Story = {
	beforeEach: () => setTouchCapability(true),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toolbar = await canvas.findByRole("toolbar", {
			name: "Terminal clipboard",
		});

		expect(within(toolbar).getByRole("button", { name: "Copy" })).toBeVisible();
		expect(
			within(toolbar).getByRole("button", { name: "Paste" }),
		).toBeVisible();
	},
};

export const CopyVisibleOutput: Story = {
	args: {
		onContentReady: fn(),
	},
	beforeEach: () => {
		const restoreTouchCapability = setTouchCapability(true);
		spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
		return restoreTouchCapability;
	},
	parameters: {
		webSocket: [{ event: "message", data: visibleOutput }],
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => expect(args.onContentReady).toHaveBeenCalled());
		await userEvent.click(canvas.getByRole("button", { name: "Copy" }));

		const body = within(document.body);
		const output = await body.findByRole("textbox", {
			name: "Visible terminal output",
		});
		expect(output).toHaveValue(`${wrappedOutput}\nSecond line`);

		await userEvent.click(body.getByRole("button", { name: "Copy" }));
		expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
			`${wrappedOutput}\nSecond line`,
		);
	},
};

export const NonTouchClipboardToolbarHidden: Story = {
	beforeEach: () => setTouchCapability(false),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.queryByRole("toolbar", { name: "Terminal clipboard" }),
			).not.toBeInTheDocument();
		});
	},
};
