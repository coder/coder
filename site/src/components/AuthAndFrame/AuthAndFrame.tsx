import { FC } from "react"
import { Footer } from "../Footer/Footer"
import { Navbar } from "../Navbar/Navbar"
import { RequireAuth } from "../RequireAuth/RequireAuth"

interface AuthAndFrameProps {
  children: JSX.Element
}

/**
 * Wraps page in RequireAuth and renders it between Navbar and Footer
 */
export const AuthAndFrame: FC<AuthAndFrameProps> = ({ children }) => (
  <RequireAuth>
    <>
      <Navbar />
      {children}
      <Footer />
    </>
  </RequireAuth>
)
