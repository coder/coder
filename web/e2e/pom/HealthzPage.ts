import { Locator, Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class HealthzPage extends BasePom {
  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, "/healthz", page)
  }

  getOk(): Locator {
    const locator = this.page.locator("text=ok")
    return locator
  }
}
