import { createContext, useContext } from "react";

interface AuthState {
  user: { name: string } | null;
}

const Auth = createContext<AuthState>({ user: { name: "hello" } });

interface AuthOptions<R extends boolean> {
  required?: R;
}

export function useAuth<R extends boolean>(
  options: AuthOptions<R> = {},
): R extends true
  ? NonNullable<Pick<AuthState, "user">> & Omit<AuthState, "user">
  : AuthState {
  const auth = useContext(Auth);

  if (options.required && !auth.user) {
    throw new Error();
  }

  return auth as unknown as any;
}

function Component() {
  // const auth = useAuth();
  const auth = useAuth({ required: true });

  const user = auth.user;
}
