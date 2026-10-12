import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockOrganizationAISpendReport } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { type SpendReportQuery, SpendUsersTable } from "./SpendUsersTable";

const requestError = new Error("Unable to load organization spend");

it("retries the initial request when the load failed", async () => {
	const refetch = vi.fn();
	const reportQuery = {
		...mockInitialRenderResult,
		data: undefined,
		isLoading: false,
		isFetching: false,
		error: requestError,
		refetch,
	} satisfies SpendReportQuery;
	render(<SpendUsersTable reportQuery={reportQuery} />);

	await userEvent.click(screen.getByRole("button", { name: "Retry" }));

	expect(refetch).toHaveBeenCalledTimes(1);
});

it("retries the refresh when a background refetch failed", async () => {
	const refetch = vi.fn();
	const reportQuery = {
		...mockSuccessResult,
		totalRecords: MockOrganizationAISpendReport.count,
		data: MockOrganizationAISpendReport,
		isLoading: false,
		isFetching: false,
		error: requestError,
		refetch,
	} satisfies SpendReportQuery;
	render(<SpendUsersTable reportQuery={reportQuery} />);

	await userEvent.click(screen.getByRole("button", { name: "Retry" }));

	expect(refetch).toHaveBeenCalledTimes(1);
});
