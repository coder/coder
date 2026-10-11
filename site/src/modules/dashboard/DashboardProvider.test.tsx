import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { useEffect } from "react";
import { describe, expect, it, vi } from "vitest";
import { experimentsKey } from "#/api/queries/experiments";
import { organizationsKey } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockExperiments,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import type { DashboardValue } from "./DashboardProvider";
import { useDashboard } from "./useDashboard";

describe("DashboardProvider", () => {
	it.each([
		{
			field: "experiments",
			path: "/api/v2/experiments",
			key: experimentsKey(MockUserOwner.id),
			expected: MockExperiments,
		},
		{
			field: "organizations",
			path: "/api/v2/organizations",
			key: organizationsKey,
			expected: [MockDefaultOrganization],
		},
	] as const)(
		"keeps the dashboard value when refetching $field fails",
		async ({ field, path, key, expected }) => {
			const onRender = vi.fn<(value: DashboardValue[typeof field]) => void>();
			const onUnmount = vi.fn();
			const Consumer: React.FC = () => {
				const dashboard = useDashboard();
				onRender(dashboard[field]);
				useEffect(() => onUnmount, []);
				return null;
			};
			const { queryClient } = renderWithAuth(<Consumer />);
			await waitFor(() => expect(onRender).toHaveBeenCalled());
			expect(onRender).toHaveBeenLastCalledWith(expected);
			const rendersBeforeRefetch = onRender.mock.calls.length;

			server.use(
				http.get(path, () =>
					HttpResponse.json({ message: "unavailable" }, { status: 500 }),
				),
			);
			await queryClient.refetchQueries({ queryKey: key });
			await waitFor(() =>
				expect(queryClient.getQueryState(key)?.status).toBe("error"),
			);

			await waitFor(() =>
				expect(onRender.mock.calls.length).toBeGreaterThan(
					rendersBeforeRefetch,
				),
			);
			expect(onRender).toHaveBeenLastCalledWith(expected);
			expect(onUnmount).not.toHaveBeenCalled();
		},
	);
});
