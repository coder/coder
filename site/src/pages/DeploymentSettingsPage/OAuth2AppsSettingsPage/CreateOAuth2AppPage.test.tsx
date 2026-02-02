import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { vi } from "vitest";
import CreateOAuth2AppPage from "./CreateOAuth2AppPage";

// Mock the GlobalSnackbar utils to prevent side effects
vi.mock("components/GlobalSnackbar/utils", () => ({
	displayError: vi.fn(),
	displaySuccess: vi.fn(),
}));

describe("CreateOAuth2AppPage", () => {
	const renderWithRouter = (initialEntry: string) => {
		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
				},
			},
		});

		const router = createMemoryRouter(
			[
				{
					path: "/deployment/oauth2-provider/apps/add",
					element: <CreateOAuth2AppPage />,
				},
			],
			{
				initialEntries: [initialEntry],
			},
		);

		return render(
			<QueryClientProvider client={queryClient}>
				<RouterProvider router={router} />
			</QueryClientProvider>,
		);
	};

	it("pre-fills form fields from URL query parameters", async () => {
		renderWithRouter(
			"/deployment/oauth2-provider/apps/add?name=TestApp&callback_url=https://test.com/callback&icon=/icon/test.svg",
		);

		await waitFor(() => {
			const nameInput = screen.getByLabelText("Application name");
			expect(nameInput).toHaveValue("TestApp");
		});

		const callbackInput = screen.getByLabelText("Callback URL");
		const iconInput = screen.getByLabelText("Application icon");

		expect(callbackInput).toHaveValue("https://test.com/callback");
		expect(iconInput).toHaveValue("/icon/test.svg");
	});

	it("renders with empty fields when no query parameters provided", async () => {
		renderWithRouter("/deployment/oauth2-provider/apps/add");

		await waitFor(() => {
			const nameInput = screen.getByLabelText("Application name");
			expect(nameInput).toHaveValue("");
		});

		const callbackInput = screen.getByLabelText("Callback URL");
		const iconInput = screen.getByLabelText("Application icon");

		expect(callbackInput).toHaveValue("");
		expect(iconInput).toHaveValue("");
	});

	it("handles URL-encoded callback URLs correctly", async () => {
		const encodedCallbackUrl = encodeURIComponent(
			"https://example.com/callback?state=123",
		);
		renderWithRouter(
			`/deployment/oauth2-provider/apps/add?name=MyApp&callback_url=${encodedCallbackUrl}`,
		);

		await waitFor(() => {
			const nameInput = screen.getByLabelText("Application name");
			expect(nameInput).toHaveValue("MyApp");
		});

		const callbackInput = screen.getByLabelText("Callback URL");
		expect(callbackInput).toHaveValue("https://example.com/callback?state=123");
	});
});
