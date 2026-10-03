import { render, screen } from "@testing-library/react";
import { Alert, AlertDescription, AlertTitle } from "./Alert";

describe("AlertTitle", () => {
	it("renders as an h2 heading", () => {
		render(
			<Alert>
				<AlertTitle>Deployment warning</AlertTitle>
				<AlertDescription>Something needs your attention.</AlertDescription>
			</Alert>,
		);

		expect(
			screen.getByRole("heading", { level: 2, name: "Deployment warning" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { level: 1, name: "Deployment warning" }),
		).not.toBeInTheDocument();
	});
});
