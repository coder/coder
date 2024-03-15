import type { Page } from "@playwright/test";
import { BasePom } from "./BasePom";

export class SignInPage extends BasePom {
  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, "/login", page);
  }

  async submitBuiltInAuthentication(
    email: string,
    password: string,
  ): Promise<void> {
    await this.page.fill("text=Email", email);
    await this.page.fill("text=Password", password);
    await this.page.click('button:has-text("Sign In")');
  }
}
