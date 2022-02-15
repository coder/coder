import { Page } from "@playwright/test"

export class SignInPage {
  private page: Page

  constructor(page: Page) {
    this.page = page
  }

  async submitBuiltInAuthentication(email: string, password: string): Promise<void> {
    await this.page.fill("id=signin-form-inpt-email", email)
    await this.page.fill("id=signin-form-inpt-password", password)
    await this.page.click("id=signin-form-submit")
  }
}
