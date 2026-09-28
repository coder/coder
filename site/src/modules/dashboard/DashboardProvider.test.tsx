import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { useEffect } from "react";
import { describe, expect, it, vi } from "vitest";
import { experimentsKey } from "#/api/queries/experiments";
import type { Experiment } from "#/api/typesGenerated";
import { MockExperiments, MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { useDashboard } from "./useDashboard";

describe("DashboardProvider", () => {
	it("keeps the dashboard value when an experiments refetch fails", async () => {
		const onRender = vi.fn<(experiments: Experiment[]) => void>();
		const onUnmount = vi.fn();
		const Consumer: React.FC = () => {
			const { experiments } = useDashboard();
			onRender(experiments);
			useEffect(() => onUnmount, []);
			return null;
		};
		const { queryClient } = renderWithAuth(<Consumer />);
		await waitFor(() => expect(onRender).toHaveBeenCalled());
		expect(onRender).toHaveBeenLastCalledWith(MockExperiments);
		const rendersBeforeRefetch = onRender.mock.calls.length;

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

		await waitFor(() =>
			expect(onRender.mock.calls.length).toBeGreaterThan(rendersBeforeRefetch),
		);
		expect(onRender).toHaveBeenLastCalledWith(MockExperiments);
		expect(onUnmount).not.toHaveBeenCalled();
	});
});
