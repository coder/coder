import React from "react"
import { Navbar } from "../Navbar"
import { RequireAuth, RequireAuthProps } from "./RequireAuth"

export const AuthAndNav: React.FC<RequireAuthProps> = ({ children }) => (
  <RequireAuth>
    <>
      <Navbar />
      {children}
    </>
  </RequireAuth>
)
