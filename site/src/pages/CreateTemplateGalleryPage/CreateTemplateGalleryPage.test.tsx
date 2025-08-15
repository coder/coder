import { AppProviders } from "App";
import {
	MockTemplateExample,
	MockTemplateExample2,
} from "testHelpers/entities";
import { server } from "testHelpers/server";
import { render, screen } from "@testing-library/react";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { HttpResponse, http } from "msw";
import { createMemoryRouter, RouterProvider } from "react-router";
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
