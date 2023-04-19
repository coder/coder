import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class CreateTemplatePage extends BasePom {
  readonly createTemplateForm: Locator
  readonly submitButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates`, page)

    this.createTemplateForm = page.getByTestId("form-create-template")
    this.submitButton = page.getByTestId("button-create-template")
  }

  async loaded() {
    await expect(this.page).toHaveTitle("Create Template - Coder")

    await this.createTemplateForm.waitFor({ state: "visible" })
    await this.submitButton.waitFor({ state: "visible" })
  }

  async submitForm() {
    await this.createTemplateForm
    .getByTestId("form-template-upload")
    .setInputFiles("./e2e/testdata/docker.tar")
    await this.createTemplateForm.getByLabel("Name *").fill("my-first-template")
    await this.createTemplateForm.getByLabel("Display name").fill("My First Template")
    await this.createTemplateForm
      .getByLabel("Description")
      .fill("This is my first template.")

    await this.submitButton.click()
  }
}
