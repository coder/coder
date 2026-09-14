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
import { withDesktopViewport } from "#/testHelpers/storybook";
import { ProviderFilter, useProviderFilterMenu } from "./ProviderFilter";

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
		// Auditors cannot read provider configuration; the filter must not
		// depend on it.
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
				screen.getByRole("option", { name: /Anthropic/ }),
			).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: /OpenAI/ }),
			).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: /GitHub Copilot/ }),
			).toBeInTheDocument();
		});
		expect(API.experimental.listAIProviders).not.toHaveBeenCalled();
	},
};

export const Preselected: Story = {
	args: { value: "anthropic" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Select provider" }),
			).toHaveTextContent("Anthropic");
		});
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
