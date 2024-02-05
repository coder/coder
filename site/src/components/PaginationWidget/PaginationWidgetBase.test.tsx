import { screen } from "@testing-library/react";
import {
  PaginationWidgetBase,
  PaginationWidgetBaseProps,
} from "./PaginationWidgetBase";
import { renderWithAuth } from "testHelpers/renderHelpers";
import userEvent from "@testing-library/user-event";

type SampleProps = Omit<PaginationWidgetBaseProps, "onPageChange">;

describe(PaginationWidgetBase.name, () => {
  it("Should have its previous button be aria-disabled while on page 1", async () => {
    const sampleProps: SampleProps[] = [
      { currentPage: 1, pageSize: 5, totalRecords: 6 },
      { currentPage: 1, pageSize: 50, totalRecords: 200 },
      { currentPage: 1, pageSize: 20, totalRecords: 3000 },
    ];

    for (const props of sampleProps) {
      const onPageChange = jest.fn();
      const { unmount } = renderWithAuth(
        <PaginationWidgetBase {...props} onPageChange={onPageChange} />,
      );

      const prevButton = await screen.findByLabelText("Previous page");
      expect(prevButton).not.toBeDisabled();
      expect(prevButton).toHaveAttribute("aria-disabled", "true");

      await userEvent.click(prevButton);
      expect(onPageChange).not.toHaveBeenCalled();
      unmount();
    }
  });

  it("Should have its next button be aria-disabled while on last page", async () => {
    const sampleProps: SampleProps[] = [
      { currentPage: 2, pageSize: 5, totalRecords: 6 },
      { currentPage: 4, pageSize: 50, totalRecords: 200 },
      { currentPage: 10, pageSize: 100, totalRecords: 1000 },
    ];

    for (const props of sampleProps) {
      const onPageChange = jest.fn();
      const { unmount } = renderWithAuth(
        <PaginationWidgetBase {...props} onPageChange={onPageChange} />,
      );

      const button = await screen.findByLabelText("Next page");
      expect(button).not.toBeDisabled();
      expect(button).toHaveAttribute("aria-disabled", "true");

      await userEvent.click(button);
      expect(onPageChange).not.toHaveBeenCalled();
      unmount();
    }
  });

  it("Should have neither button be disabled for all other pages", async () => {
    const sampleProps: SampleProps[] = [
      { currentPage: 11, pageSize: 5, totalRecords: 60 },
      { currentPage: 2, pageSize: 50, totalRecords: 200 },
      { currentPage: 3, pageSize: 20, totalRecords: 100 },
    ];

    for (const props of sampleProps) {
      const onPageChange = jest.fn();
      const { unmount } = renderWithAuth(
        <PaginationWidgetBase {...props} onPageChange={onPageChange} />,
      );

      const prevButton = await screen.findByLabelText("Previous page");
      const nextButton = await screen.findByLabelText("Next page");

      expect(prevButton).not.toBeDisabled();
      expect(prevButton).toHaveAttribute("aria-disabled", "false");

      await userEvent.click(prevButton);
      expect(onPageChange).toHaveBeenCalledTimes(1);

      expect(nextButton).not.toBeDisabled();
      expect(nextButton).toHaveAttribute("aria-disabled", "false");

      await userEvent.click(nextButton);
      expect(onPageChange).toHaveBeenCalledTimes(2);

      unmount();
    }
  });
});
