import { Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class TemplatesPage extends BasePom {
  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, "/templates", page)
  }
}
