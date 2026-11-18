import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { useQueryClient } from "react-query";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { DefaultLandingPageSettings } from "./DefaultLandingPageSettings";

const meta = {
	title: "pages/UserSettingsPage/DefaultLandingPageSettings",
	component: DefaultLandingPageSettings,
	args: { agentsEnabled: true },
	decorators: [
		withToaster,
		(Story) => (
			<div className="w-full max-w-3xl">
				<Story />
			</div>
		),
	],
	parameters: { layout: "padded" },
} satisfies Meta<typeof DefaultLandingPageSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Workspaces: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
};

export const Agents: Story = {
	parameters: {
		queries: [
			{
				key: preferenceSettingsKey,
				data: { ...MockUserPreferenceSettings, default_landing_page: "agents" },
			},
		],
	},
};

export const AgentsUnavailable: Story = {
	args: { agentsEnabled: false },
	parameters: {
		queries: [
			{
				key: preferenceSettingsKey,
				data: { ...MockUserPreferenceSettings, default_landing_page: "agents" },
			},
		],
	},
};

export const OptionsOpen: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		await user.click(
			await within(canvasElement).findByRole("combobox", {
				name: "Default landing page",
			}),
		);
		await user.keyboard("{ArrowDown}");
	},
};

export const Loading: Story = {
	beforeEach: () => {
		const request = spyOn(API, "getUserPreferenceSettings").mockReturnValue(
			createDeferred<UserPreferenceSettings>().promise,
		);
		return () => request.mockRestore();
	},
};

export const Refetching: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	decorators: [
		(Story) => {
			const queryClient = useQueryClient();
			useEffect(() => {
				void queryClient.invalidateQueries({
					queryKey: preferenceSettingsKey,
					exact: true,
				});
			}, [queryClient]);
			return <Story />;
		},
	],
	beforeEach: () => {
		const request = spyOn(API, "getUserPreferenceSettings").mockReturnValue(
			createDeferred<UserPreferenceSettings>().promise,
		);
		return () => request.mockRestore();
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByText("Loading default landing page");
	},
};

export const Saving: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	beforeEach: () => {
		const request = spyOn(API, "updateUserPreferenceSettings").mockReturnValue(
			createDeferred<UserPreferenceSettings>().promise,
		);
		return () => request.mockRestore();
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		await user.click(
			await within(canvasElement).findByRole("combobox", {
				name: "Default landing page",
			}),
		);
		await user.click(await screen.findByRole("option", { name: "Agents" }));
	},
};

export const Saved: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	beforeEach: () => {
		const saved: UserPreferenceSettings = {
			...MockUserPreferenceSettings,
			default_landing_page: "agents",
		};
		const getPreference = spyOn(
			API,
			"getUserPreferenceSettings",
		).mockResolvedValue(saved);
		const updatePreference = spyOn(
			API,
			"updateUserPreferenceSettings",
		).mockResolvedValue(saved);
		return () => {
			getPreference.mockRestore();
			updatePreference.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		await user.click(
			await within(canvasElement).findByRole("combobox", {
				name: "Default landing page",
			}),
		);
		await user.click(await screen.findByRole("option", { name: "Agents" }));
		await screen.findByText("Default landing page updated.");
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		const request = spyOn(API, "getUserPreferenceSettings").mockRejectedValue(
			mockApiError({ message: "Unable to load your default landing page." }),
		);
		return () => request.mockRestore();
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByText("Workspaces");
	},
};

export const SaveError: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	beforeEach: () => {
		const request = spyOn(
			API,
			"updateUserPreferenceSettings",
		).mockRejectedValue(
			mockApiError({ message: "Unable to save your default landing page." }),
		);
		return () => request.mockRestore();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await user.click(
			await canvas.findByRole("combobox", { name: "Default landing page" }),
		);
		await user.click(await screen.findByRole("option", { name: "Agents" }));
		await canvas.findByText("Unable to save your default landing page.");
	},
};
