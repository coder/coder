import { Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class ProjectsPage extends BasePom {
  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, "/projects", page)
  }
}
