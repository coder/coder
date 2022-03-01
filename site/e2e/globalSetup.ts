import { FullConfig, request } from "@playwright/test"

const globalSetup = async (config: FullConfig): Promise<void> => {
  // Grab the 'baseURL' from the webserver (`coderd`)
  const { baseURL } = config.projects[0].use

  // Create a context that will issue http requests.
  const context = await request.newContext({
    baseURL,
  })

  // Create initial user
  await context.post("/api/v2/user", {
    data: {
      email: "admin@coder.com",
      username: "admin",
      password: "password",
      organization: "acme-corp",
    },
  })
}

export default globalSetup
