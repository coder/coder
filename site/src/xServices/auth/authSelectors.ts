import { State } from "xstate"
import { AuthContext, AuthEvent } from "./authXService"

export const selectOrgId = (state: State<AuthContext, AuthEvent>): string | undefined => {
  return state.context.me?.organization_ids[0]
}
