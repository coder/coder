import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18next from "i18next";
import { render } from "testHelpers/renderHelpers";
import { DeleteDialog } from "./DeleteDialog";

describe("DeleteDialog", () => {
  it("disables confirm button when the text field is empty", () => {
    render(
      <DeleteDialog
        isOpen
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
        entity="template"
        name="MyTemplate"
      />,
    );
    const confirmButton = screen.getByRole("button", { name: "Delete" });
    expect(confirmButton).toBeDisabled();
  });

  it("disables confirm button when the text field is filled incorrectly", async () => {
    const { t } = i18next;
    render(
      <DeleteDialog
        isOpen
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
        entity="template"
        name="MyTemplate"
      />,
    );
    const labelText = t("deleteDialog.confirmLabel", {
      ns: "common",
      entity: "template",
    });
    const textField = screen.getByLabelText(labelText);
    await userEvent.type(textField, "MyTemplateWrong");
    const confirmButton = screen.getByRole("button", { name: "Delete" });
    expect(confirmButton).toBeDisabled();
  });

  it("enables confirm button when the text field is filled correctly", async () => {
    const { t } = i18next;
    render(
      <DeleteDialog
        isOpen
        onConfirm={jest.fn()}
        onCancel={jest.fn()}
        entity="template"
        name="MyTemplate"
      />,
    );
    const labelText = t("deleteDialog.confirmLabel", {
      ns: "common",
      entity: "template",
    });
    const textField = screen.getByLabelText(labelText);
    await userEvent.type(textField, "MyTemplate");
    const confirmButton = screen.getByRole("button", { name: "Delete" });
    expect(confirmButton).not.toBeDisabled();
  });
});
