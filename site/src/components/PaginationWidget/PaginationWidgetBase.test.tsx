import { screen } from "@testing-library/react";
import {
  PaginationWidgetBase,
  PaginationWidgetBaseProps,
} from "./PaginationWidgetBase";
import { renderWithAuth } from "testHelpers/renderHelpers";
import userEvent from "@testing-library/user-event";

type SampleProps = Omit<PaginationWidgetBaseProps, "onPageChange">;
const sampleProps: SampleProps[] = [
  { currentPage: 1, pageSize: 5, totalRecords: 6 },
  { currentPage: 1, pageSize: 50, totalRecords: 200 },
  { currentPage: 2, pageSize: 20, totalRecords: 3000 },
];

describe(PaginationWidgetBase.name, () => {
  it("Should have its previous button be aria-disabled while on page 1", async () => {
    for (const props of sampleProps) {
      const onPageChange = jest.fn();

      const { unmount } = renderWithAuth(
        <PaginationWidgetBase
          {...props}
          currentPage={1}
          onPageChange={onPageChange}
        />,
      );

      const button = await screen.findByLabelText("Previous page");
      expect(button).not.toBeDisabled();
      expect(button).toHaveAttribute("aria-disabled", "true");

      await userEvent.click(button);
      expect(onPageChange).not.toHaveBeenCalled();
      unmount();
    }
  });

  it("Should have its next button be aria-disabled while on last page", async () => {
    for (const props of sampleProps) {
      const onPageChange = jest.fn();
      const lastPage = Math.ceil(props.totalRecords / props.pageSize);

      const { unmount } = renderWithAuth(
        <PaginationWidgetBase
          {...props}
          currentPage={lastPage}
          onPageChange={onPageChange}
        />,
      );

      const button = await screen.findByLabelText("Next page");
      expect(button).not.toBeDisabled();
      expect(button).toHaveAttribute("aria-disabled", "true");

      await userEvent.click(button);
      expect(onPageChange).not.toHaveBeenCalled();
      unmount();
    }
  });
});
