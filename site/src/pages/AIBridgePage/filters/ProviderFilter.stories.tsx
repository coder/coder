import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import {
	MockAIProviderAnthropic,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { withDesktopViewport } from "#/testHelpers/storybook";
import { ProviderFilter, useProviderFilterMenu } from "./ProviderFilter";

const providerNames = [MockAIProviderOpenAI.name, MockAIProviderAnthropic.name];

function ProviderFilterWithMenu({ value: initialValue }: { value?: string }) {
	const [value, setValue] = useState(initialValue);
	const menu = useProviderFilterMenu({
		value,
		onChange: (option) => setValue(option?.value),
	});
	return <ProviderFilter menu={menu} />;
}

const meta = {
	title: "pages/AIBridgePage/ProviderFilter",
	component: ProviderFilterWithMenu,
	decorators: [withDesktopViewport],
	beforeEach: () => {
		spyOn(API, "getAIBridgeProviders").mockResolvedValue(providerNames);
		spyOn(API.experimental, "listAIProviders").mockRejectedValue(
			new Error("listAIProviders should not be called"),
		);
	},
} satisfies Meta<typeof ProviderFilterWithMenu>;

export default meta;
type Story = StoryObj<typeof ProviderFilterWithMenu>;

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Select provider" }),
		);
		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: /OpenAI/ }),
			).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: /Anthropic/ }),
			).toBeInTheDocument();
		});
		expect(API.getAIBridgeProviders).toHaveBeenCalled();
	},
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API, "getAIBridgeProviders").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Select provider" }),
		);
		await waitFor(() => {
			expect(screen.getByText("No providers found")).toBeVisible();
		});
		expect(screen.queryByRole("option")).toBeNull();
	},
};

export const SelectingOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: "Select provider" });
		await userEvent.click(button);
		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: /OpenAI/ }),
			).toBeInTheDocument();
		});
		await userEvent.click(screen.getByRole("option", { name: /OpenAI/ }));
		await waitFor(() => {
			expect(button).toHaveTextContent("OpenAI");
		});
	},
};

export const Searching: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Select provider" }),
		);
		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: /OpenAI/ }),
			).toBeInTheDocument();
		});
		await userEvent.type(
			screen.getByPlaceholderText("Search provider..."),
			"anthropic",
		);
		await waitFor(() => {
			expect(API.getAIBridgeProviders).toHaveBeenCalledWith(
				expect.objectContaining({ q: "anthropic" }),
			);
		});
	},
};
