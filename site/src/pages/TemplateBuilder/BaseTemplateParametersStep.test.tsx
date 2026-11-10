import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { TemplateBuilderBasesResponse } from "#/api/typesGenerated";
import { render } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { BaseTemplateParametersStep } from "./BaseTemplateParametersStep";

describe(BaseTemplateParametersStep.name, () => {
	it("nests prerequisites headings under the base template heading", async () => {
		const bases: TemplateBuilderBasesResponse = {
			bases: [
				{
					id: "docker",
					name: "Docker",
					description: "Docker containers.",
					icon: "/icon/docker.svg",
					os: "linux",
					prerequisites: "## Prerequisites\n\n### Infrastructure\n\nA host.",
					agents: [],
					variables: [],
				},
			],
		};
		server.use(
			http.get("/api/v2/templatebuilder/bases", () => HttpResponse.json(bases)),
		);
		render(
			<BaseTemplateParametersStep
				baseId="docker"
				values={{}}
				onChangeValues={vi.fn()}
			/>,
		);

		await screen.findByText("Prerequisites");
		expect(
			screen.getAllByRole("heading").map((h) => [h.tagName, h.textContent]),
		).toEqual([
			["H2", "Configure base template"],
			["H3", "Docker"],
			["H4", "Prerequisites"],
			["H5", "Infrastructure"],
		]);
	});
});
