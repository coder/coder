import { State } from "xstate"
import { AuthContext, AuthEvent } from "./authXService"

export const selectOrgId = (state: State<AuthContext, AuthEvent>) => {
  return state.context.me?.organization_id
}
