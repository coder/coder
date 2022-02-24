import React from "react";
/*import {
  BrowserRouter as Router,
  Switch,
  Route,
  Link,
  useParams
} from "react-router-dom"*/
import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { SWRConfig } from "swr"
//import { AppProps } from "next/app"
import { UserProvider } from "./contexts/UserContext"
import { light } from "./theme"

export const App: React.FC = () => {
  return <SWRConfig
    value={{
      fetcher: async (url: string) => {
        const res = await fetch(url)
        if (!res.ok) {
          const err = new Error((await res.json()).error?.message || res.statusText)
          throw err
        }
        return res.json()
      },
    }}
  >
    <UserProvider>
      <ThemeProvider theme={light}>
        <CssBaseline />
        <div>Hello, world</div>
      </ThemeProvider>
    </UserProvider>
  </SWRConfig>
}