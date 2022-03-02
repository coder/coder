import { Page } from "@playwright/test"

export abstract class BasePom {
  abstract get url(): string

  protected page: Page
  protected baseURL: string | undefined

  constructor(baseURL: string | undefined, page: Page) {
    this.page = page
    this.baseURL = baseURL
  }
}
