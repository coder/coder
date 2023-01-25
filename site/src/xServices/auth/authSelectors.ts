import { StateFrom } from "xstate"
import { AuthContext, authMachine } from "./authXService"

type AuthState = StateFrom<typeof authMachine>

export const selectOrgId = (state: AuthState): string | undefined => {
  return state.context.me?.organization_ids[0]
}

export const selectPermissions = (
  state: AuthState,
): AuthContext["permissions"] => {
  return state.context.permissions
}

export const selectUser = (state: AuthState): AuthContext["me"] => {
  return state.context.me
}
