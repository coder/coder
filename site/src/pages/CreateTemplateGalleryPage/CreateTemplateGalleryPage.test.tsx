import { render, screen } from "@testing-library/react";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { http, HttpResponse } from "msw";
import { RouterProvider, createMemoryRouter } from "react-router-dom";
import {
	MockTemplateExample,
	MockTemplateExample2,
} from "testHelpers/entities";
import { server } from "testHelpers/server";
import CreateTemplateGalleryPage from "./CreateTemplateGalleryPage";

test("displays the scratch template", async () => {
	server.use(
		http.get("api/v2/templates/examples", () => {
			return HttpResponse.json([
				MockTemplateExample,
				MockTemplateExample2,
				{
					...MockTemplateExample,
					id: "scratch",
					name: "Scratch",
					description: "Create a template from scratch",
				},
			]);
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
									path: "/starter-templates",
									element: <CreateTemplateGalleryPage />,
								},
							],
						},
					],
					{ initialEntries: ["/starter-templates"] },
				)}
			/>
		</AppProviders>,
	);

	await screen.findByText(MockTemplateExample.name);
	screen.getByText(MockTemplateExample2.name);
	expect(screen.queryByText("Scratch")).toBeInTheDocument();
});
