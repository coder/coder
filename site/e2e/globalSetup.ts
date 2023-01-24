import axios from "axios"
import { request } from "playwright"
import { createFirstUser } from "../src/api/api"
import * as constants from "./constants"
import { getStatePath } from "./helpers"

const globalSetup = async (): Promise<void> => {
  axios.defaults.baseURL = `http://localhost:${constants.defaultPort}`

  // Create first user
  await createFirstUser({
    email: constants.email,
    username: constants.username,
    password: constants.password,
    trial: false,
  })

  // Authenticated storage
  const authenticatedRequestContext = await request.newContext()
  await authenticatedRequestContext.post(
    `http://localhost:${constants.defaultPort}/api/v2/users/login`,
    {
      data: {
        email: constants.email,
        password: constants.password,
      },
    },
  )
  await authenticatedRequestContext.storageState({
    path: getStatePath("authState"),
  })
  await authenticatedRequestContext.dispose()
}

export default globalSetup
