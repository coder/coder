import { useActor, useInterpret } from "@xstate/react"
import { createContext, FC, PropsWithChildren, useContext } from "react"
import { authMachine } from "xServices/auth/authXService"
import { ActorRefFrom } from "xstate"

interface AuthProviderContextValue {
  authService: ActorRefFrom<typeof authMachine>
}

const AuthProviderContext = createContext<AuthProviderContextValue | undefined>(
  undefined,
)

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const authService = useInterpret(authMachine)

  return (
    <AuthProviderContext.Provider value={{ authService }}>
      {children}
    </AuthProviderContext.Provider>
  )
}

type UseAuthReturnType = ReturnType<
  typeof useActor<AuthProviderContextValue["authService"]>
>

export const useAuth = (): UseAuthReturnType => {
  const context = useContext(AuthProviderContext)

  if (!context) {
    throw new Error("useAuth should be used inside of <AuthProvider />")
  }

  const auth = useActor(context.authService)

  return auth
}
