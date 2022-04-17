import React from "react"
import { Navbar } from "../Navbar"
import { Footer } from "../Page/Footer"
import { RequireAuth } from "../Page/RequireAuth"

interface AuthAndFrameProps {
  children: JSX.Element
}

/**
 * Wraps page in RequireAuth and renders it between Navbar and Footer
 */
export const AuthAndFrame: React.FC<AuthAndFrameProps> = ({ children }) => (
  <RequireAuth>
    <>
      <Navbar />
      {children}
      <Footer />
    </>
  </RequireAuth>
)
