import { State } from "xstate"
import { AuthContext, AuthEvent } from "./authXService"

type AuthState = State<AuthContext, AuthEvent>

export const selectOrgId = (state: AuthState): string | undefined => {
  return state.context.me?.organization_ids[0]
}

export const selectPermissions = (state: AuthState): AuthContext["permissions"] => {
  return state.context.permissions
}

export const selectUser = (state: AuthState): AuthContext["me"] => {
  return state.context.me
}
