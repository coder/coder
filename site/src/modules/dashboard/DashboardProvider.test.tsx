import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { experimentsKey } from "#/api/queries/experiments";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";

describe("DashboardProvider", () => {
	it("keeps the dashboard when an experiments refetch fails", async () => {
		const { queryClient } = renderWithAuth(<h1>Dashboard content</h1>);
		await screen.findByText("Dashboard content");

		// Experiments refetch in the background. A failed refetch must keep
		// the last list instead of replacing the dashboard with an error.
		server.use(
			http.get("/api/v2/experiments", () =>
				HttpResponse.json({ message: "unavailable" }, { status: 500 }),
			),
		);
		const key = experimentsKey(MockUserOwner.id);
		await queryClient.refetchQueries({ queryKey: key });
		await waitFor(() =>
			expect(queryClient.getQueryState(key)?.status).toBe("error"),
		);

		expect(screen.getByText("Dashboard content")).toBeInTheDocument();
	});
});
