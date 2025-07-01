import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Response, User } from "api/typesGenerated";
import { http, HttpResponse } from "msw";
import { createMemoryRouter } from "react-router-dom";
import { MockBuildInfo, MockUserOwner } from "testHelpers/entities";
import {
	renderWithRouter,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { SetupPage } from "./SetupPage";
import { Language as PageViewLanguage } from "./SetupPageView";

const fillForm = async ({
	email = "someone@coder.com",
	password = "password",
}: {
	username?: string;
	email?: string;
	password?: string;
} = {}) => {
	const emailField = screen.getByLabelText(PageViewLanguage.emailLabel);
	const passwordField = screen.getByLabelText(PageViewLanguage.passwordLabel);
	await userEvent.type(emailField, email);
	await userEvent.type(passwordField, password);
	const submitButton = screen.getByRole("button", {
		name: PageViewLanguage.create,
	});
	await userEvent.click(submitButton);
};

describe("Setup Page", () => {
	beforeEach(() => {
		// appear logged out
		server.use(
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "no user here" }, { status: 401 });
			}),
			http.get("/api/v2/users/first", () => {
				return HttpResponse.json(
					{ message: "no first user has been created" },
					{ status: 404 },
				);
			}),
		);
	});

	it("renders the password validation error", async () => {
		server.use(
			http.post("/api/v2/users/validate-password", () => {
				return HttpResponse.json({
					valid: false,
					details: "Password is too short",
				});
			}),
		);

		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/setup",
						element: <SetupPage />,
					},
				],
				{ initialEntries: ["/setup"] },
			),
		);
		await waitForLoaderToBeRemoved();
		await fillForm({ password: "short" });
		await waitFor(() => screen.findByText("Password is too short"));
	});

	it("redirects to the app when setup is successful", async () => {
		let userHasBeenCreated = false;

		server.use(
			http.get<never, null, User | Response>("/api/v2/users/me", async () => {
				if (!userHasBeenCreated) {
					return HttpResponse.json(
						{ message: "no user here" },
						{ status: 401 },
					);
				}
				return HttpResponse.json(MockUserOwner);
			}),
			http.get<never, null, User | Response>(
				"/api/v2/users/first",
				async () => {
					if (!userHasBeenCreated) {
						return HttpResponse.json(
							{ message: "no first user has been created" },
							{ status: 404 },
						);
					}
					return HttpResponse.json({ message: "hooray, someone exists!" });
				},
			),
			http.post("/api/v2/users/first", () => {
				userHasBeenCreated = true;
				return HttpResponse.json({ data: "user setup was successful!" });
			}),
		);

		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/setup",
						element: <SetupPage />,
					},
					{
						path: "/templates",
						element: <h1>Templates</h1>,
					},
				],
				{ initialEntries: ["/setup"] },
			),
		);
		await waitForLoaderToBeRemoved();
		await fillForm();
		await waitFor(() => screen.findByText("Templates"));
	});

	it("calls sendBeacon with telemetry", async () => {
		const sendBeacon = jest.fn();
		Object.defineProperty(window.navigator, "sendBeacon", {
			value: sendBeacon,
		});
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/setup",
						element: <SetupPage />,
					},
					{
						path: "/templates",
						element: <h1>Templates</h1>,
					},
				],
				{ initialEntries: ["/setup"] },
			),
		);
		await waitForLoaderToBeRemoved();
		await waitFor(() => {
			expect(navigator.sendBeacon).toBeCalledWith(
				"https://coder.com/api/track-deployment",
				new Blob(
					[
						JSON.stringify({
							type: "deployment_setup",
							deployment_id: MockBuildInfo.deployment_id,
						}),
					],
					{
						type: "application/json",
					},
				),
			);
		});
	});
});
