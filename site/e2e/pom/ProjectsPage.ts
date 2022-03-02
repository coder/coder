import { Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class ProjectsPage extends BasePom {
  public get url(): string {
    return this.baseURL + "/projects"
  }

  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, page)
  }
}
