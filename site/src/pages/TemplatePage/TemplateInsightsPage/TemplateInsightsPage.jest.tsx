import { AppProviders } from "App";
import { MockTemplate } from "testHelpers/entities";
import { server } from "testHelpers/server";
import { render, screen } from "@testing-library/react";
import type { Entitlements } from "api/typesGenerated";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { HttpResponse, http } from "msw";
import { createMemoryRouter, RouterProvider } from "react-router";
import { TemplateLayout } from "../TemplateLayout";
import TemplateInsightsPage from "./TemplateInsightsPage";

test("renders without crashing when user_limit feature is missing", async () => {
	server.use(
		http.get("/api/v2/entitlements", () => {
			return HttpResponse.json({
				features: {},
			} as Partial<Entitlements>);
		}),
		http.get("/api/v2/insights/templates", () => {
			return HttpResponse.json({ interval_reports: [], report: {} });
		}),
		http.get("/api/v2/insights/user-latency", () => {
			return HttpResponse.json({ report: {} });
		}),
		http.get("/api/v2/insights/user-activity", () => {
			return HttpResponse.json({ report: {} });
		}),
	);

	render(
		<AppProviders>
			<RouterProvider
				router={createMemoryRouter(
					[
						{
							element: <RequireAuth />,
							children: [
								{
									element: <TemplateLayout />,
									children: [
										{
											path: "/templates/:template/insights",
											element: <TemplateInsightsPage />,
										},
									],
								},
							],
						},
					],
					{ initialEntries: [`/templates/${MockTemplate.name}/insights`] },
				)}
			/>
		</AppProviders>,
	);

	await screen.findByText(/Active Users/);
});
