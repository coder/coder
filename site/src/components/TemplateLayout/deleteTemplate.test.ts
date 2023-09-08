import { act, renderHook, waitFor } from "@testing-library/react";
import { MockTemplate } from "testHelpers/entities";
import { useDeleteTemplate } from "./deleteTemplate";
import * as API from "api/api";

test("delete dialog starts closed", () => {
  const { result } = renderHook(() =>
    useDeleteTemplate(MockTemplate, jest.fn()),
  );
  expect(result.current.isDeleteDialogOpen).toBeFalsy();
});

test("confirm template deletion", async () => {
  const onDeleteTemplate = jest.fn();
  const { result } = renderHook(() =>
    useDeleteTemplate(MockTemplate, onDeleteTemplate),
  );

  //Open delete confirmation
  act(() => {
    result.current.openDeleteConfirmation();
  });
  expect(result.current.isDeleteDialogOpen).toBeTruthy();

  // Confirm delete
  jest.spyOn(API, "deleteTemplate");
  await act(async () => result.current.confirmDelete());
  await waitFor(() => expect(API.deleteTemplate).toBeCalledTimes(1));
  expect(onDeleteTemplate).toBeCalledTimes(1);
});

test("cancel template deletion", () => {
  const { result } = renderHook(() =>
    useDeleteTemplate(MockTemplate, jest.fn()),
  );

  //Open delete confirmation
  act(() => {
    result.current.openDeleteConfirmation();
  });
  expect(result.current.isDeleteDialogOpen).toBeTruthy();

  // Cancel deletion
  act(() => {
    result.current.cancelDeleteConfirmation();
  });
  expect(result.current.isDeleteDialogOpen).toBeFalsy();
});
