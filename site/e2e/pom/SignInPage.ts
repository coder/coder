import { Page } from "@playwright/test"
import { BasePom } from "./BasePom"

export class SignInPage extends BasePom {
  constructor(baseURL: string | undefined, page: Page) {
    super(baseURL, "/login", page)
  }

  async submitBuiltInAuthentication(email: string, password: string): Promise<void> {
    await this.page.fill("id=signin-form-inpt-email", email)
    await this.page.fill("id=signin-form-inpt-password", password)
    await this.page.click("id=signin-form-submit")
  }
}
