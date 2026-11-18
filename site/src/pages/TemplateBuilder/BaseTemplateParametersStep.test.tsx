import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { TemplateBuilderBasesResponse } from "#/api/typesGenerated";
import { render } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { BaseTemplateParametersStep } from "./BaseTemplateParametersStep";

describe(BaseTemplateParametersStep.name, () => {
	it("nests prerequisites headings under the base template heading", async () => {
		const mockBases: TemplateBuilderBasesResponse = {
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
			http.get("/api/v2/templatebuilder/bases", () =>
				HttpResponse.json(mockBases),
			),
		);
		render(
			<BaseTemplateParametersStep
				baseId="docker"
				values={{}}
				onChangeValues={vi.fn()}
			/>,
		);

		await screen.findByText("Prerequisites");
		const names = (level: number) =>
			screen.getAllByRole("heading", { level }).map((h) => h.textContent);
		expect(names(3)).toEqual(["Docker"]);
		expect(names(4)).toEqual(["Prerequisites"]);
		expect(names(5)).toEqual(["Infrastructure"]);
	});
});
