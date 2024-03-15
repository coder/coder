import type { Page } from "@playwright/test";

export abstract class BasePom {
  protected readonly baseURL: string | undefined;
  protected readonly path: string;
  protected readonly page: Page;

  constructor(baseURL: string | undefined, path: string, page: Page) {
    this.baseURL = baseURL;
    this.path = path;
    this.page = page;
  }

  get url(): string {
    return this.baseURL + this.path;
  }
}
