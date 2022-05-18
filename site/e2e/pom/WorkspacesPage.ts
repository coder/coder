import { Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class WorkspacesPage extends BasePom {
  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, "/workspaces", page)
  }
}
