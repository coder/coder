import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { OAuth2ProviderAppsResponse } from "#/api/typesGenerated";
import type { UseFilterResult } from "#/components/Filter/Filter";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockOAuth2ProviderApps, mockApiError } from "#/testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const defaultFilter: UseFilterResult = {
	query: "",
	values: {},
	update: () => {},
	debounceUpdate: () => {},
	cancelDebounce: () => {},
	used: false,
};

const appsQuerySuccess: PaginationResult<OAuth2ProviderAppsResponse> = {
	...mockSuccessResult,
	totalRecords: MockOAuth2ProviderApps.length,
	data: {
		apps: MockOAuth2ProviderApps,
		count: MockOAuth2ProviderApps.length,
	},
};

const meta: Meta<typeof OAuth2AppsSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/OAuth2AppsSettingsPageView",
	component: OAuth2AppsSettingsPageView,
	args: {
		canCreateApp: true,
		filter: defaultFilter,
		apps: MockOAuth2ProviderApps,
		appsQuery: appsQuerySuccess,
		isLoading: false,
		error: undefined,
	},
};

export default meta;

type Story = StoryObj<typeof OAuth2AppsSettingsPageView>;

export const Loading: Story = {
	args: {
		isLoading: true,
		apps: undefined,
		appsQuery: {
			...mockInitialRenderResult,
			data: undefined,
		},
	},
};

export const WithError: Story = {
	args: {
		isLoading: false,
		apps: undefined,
		error: mockApiError({
			message: "Failed to load OAuth2 applications.",
		}),
		appsQuery: {
			...mockInitialRenderResult,
			data: undefined,
		},
	},
};

export const Apps: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("table", { name: "OAuth2 applications" }),
		).toBeVisible();
		await expect(canvas.getByText("foo")).toBeVisible();
		await expect(canvas.getByText("bar")).toBeVisible();
		await expect(canvas.getByText("baz")).toBeVisible();
		await expect(canvas.getByRole("textbox", { name: "Filter" })).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /add application/i }),
		).toBeVisible();
	},
};

export const Empty: Story = {
	args: {
		isLoading: false,
		apps: [],
		appsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				apps: [],
				count: 0,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("No OAuth2 applications configured"),
		).toBeVisible();
		await expect(
			canvas.getAllByRole("link", { name: /add application/i }).length,
		).toBeGreaterThan(0);
	},
};

export const EmptySearch: Story = {
	args: {
		isLoading: false,
		apps: [],
		filter: {
			...defaultFilter,
			query: "nonexistent",
			used: true,
		},
		appsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				apps: [],
				count: 0,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("No OAuth2 applications match your search"),
		).toBeVisible();
		await expect(
			canvas.getByText("Try adjusting your search query."),
		).toBeVisible();
	},
};

export const NoCreatePermissions: Story = {
	args: {
		canCreateApp: false,
		apps: [],
		appsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				apps: [],
				count: 0,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("No OAuth2 applications configured"),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: /add application/i }),
		).not.toBeInTheDocument();
	},
};

export const Paginated: Story = {
	args: {
		apps: MockOAuth2ProviderApps.slice(0, 2),
		appsQuery: {
			...mockSuccessResult,
			currentPage: 1,
			totalPages: 2,
			totalRecords: 5,
			hasNextPage: true,
			hasPreviousPage: false,
			currentOffsetStart: 1,
			limit: 2,
			data: {
				apps: MockOAuth2ProviderApps.slice(0, 2),
				count: 5,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("foo")).toBeVisible();
		await expect(canvas.getByText("bar")).toBeVisible();
		await expect(canvas.queryByText("baz")).not.toBeInTheDocument();
		await expect(canvas.getByText(/Showing/i)).toBeVisible();
	},
};

export const SearchInteraction: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const filterInput = canvas.getByRole("textbox", { name: "Filter" });
		await userEvent.clear(filterInput);
		await userEvent.type(filterInput, "foo");
		await expect(filterInput).toHaveValue("foo");
	},
};
