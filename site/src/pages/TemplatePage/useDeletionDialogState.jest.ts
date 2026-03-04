import { MockTemplate } from "testHelpers/entities";
import { act, renderHook, waitFor } from "@testing-library/react";
import { API } from "api/api";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { useDeletionDialogState } from "./useDeletionDialogState";

const wrapper = ({ children }: { children: React.ReactNode }) =>
	createElement(QueryClientProvider, { client: new QueryClient() }, children);

test("delete dialog starts closed", () => {
	const { result } = renderHook(
		() => useDeletionDialogState(MockTemplate.id, jest.fn()),
		{ wrapper },
	);
	expect(result.current.isDeleteDialogOpen).toBeFalsy();
});

test("confirm template deletion", async () => {
	const onDeleteTemplate = jest.fn();
	const { result } = renderHook(
		() => useDeletionDialogState(MockTemplate.id, onDeleteTemplate),
		{ wrapper },
	);

	act(() => {
		result.current.openDeleteConfirmation();
	});
	expect(result.current.isDeleteDialogOpen).toBeTruthy();

	jest.spyOn(API, "deleteTemplate");
	await act(async () => result.current.confirmDelete());
	await waitFor(() => expect(API.deleteTemplate).toBeCalledTimes(1));
	await waitFor(() => expect(onDeleteTemplate).toBeCalledTimes(1));
});

test("cancel template deletion", () => {
	const { result } = renderHook(
		() => useDeletionDialogState(MockTemplate.id, jest.fn()),
		{ wrapper },
	);

	act(() => {
		result.current.openDeleteConfirmation();
	});
	expect(result.current.isDeleteDialogOpen).toBeTruthy();

	act(() => {
		result.current.cancelDeleteConfirmation();
	});
	expect(result.current.isDeleteDialogOpen).toBeFalsy();
});
