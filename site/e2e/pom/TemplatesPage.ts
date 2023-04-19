import { expect, Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class TemplatesPage extends BasePom {
  readonly addTemplateButton: Locator

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, `/templates`, page)

    this.addTemplateButton = page.getByTestId("button-add-template")
  }

  async goto() {
    await this.page.goto(this.url, { waitUntil: "networkidle" })
  }

  async loaded() {
    await this.addTemplateButton.waitFor({ state: "visible" })

    await expect(this.page).toHaveTitle("Templates - Coder")
  }

  async addTemplate() {
    await this.addTemplateButton.click()
  }
}
