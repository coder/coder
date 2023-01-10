import axios from "axios"
import { createFirstUser } from "../src/api/api"
import * as constants from "./constants"

const globalSetup = async (): Promise<void> => {
  axios.defaults.baseURL = `http://localhost:${constants.basePort}`
  // Create a user
  await createFirstUser({
    email: constants.email,
    username: constants.username,
    password: constants.password,
    trial: false,
  })
}

export default globalSetup
