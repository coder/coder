import axios from "axios"
import { postFirstUser } from "../src/api/api"
import * as constants from "./constants"

const globalSetup = async (): Promise<void> => {
  axios.defaults.baseURL = `http://localhost:${constants.basePort}`
  await postFirstUser({
    email: constants.email,
    organization: constants.organization,
    username: constants.username,
    password: constants.password,
  })
}

export default globalSetup
