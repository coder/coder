import { render, screen } from "@testing-library/react";
import { Alert, AlertDetail, AlertTitle } from "./Alert";

describe("AlertTitle", () => {
	it("renders as an h2 heading", () => {
		render(
			<Alert>
				<AlertTitle>Deployment warning</AlertTitle>
				<AlertDetail>Something needs your attention.</AlertDetail>
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
