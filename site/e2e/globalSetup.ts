import { FullConfig, request } from "@playwright/test"

async function globalSetup(config: FullConfig) {
  const { baseURL } = config.projects[0].use
  console.log(" -- Starting global setup: " + baseURL)

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

  console.log("-- Created initial user.")
}

export default globalSetup
