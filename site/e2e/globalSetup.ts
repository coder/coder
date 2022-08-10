import axios from "axios"
import { createFirstUser } from "../src/api/api"
import * as constants from "./constants"

const globalSetup = async (): Promise<void> => {
  axios.defaults.baseURL = `http://localhost:${constants.basePort}`
  await createFirstUser({
    email: constants.email,
    organization: constants.organization,
    username: constants.username,
    password: constants.password,
  })
}

export default globalSetup
