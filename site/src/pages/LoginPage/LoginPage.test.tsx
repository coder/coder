import { fireEvent, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { rest } from "msw"
import { Route, Routes } from "react-router-dom"
import { Language } from "../../components/SignInForm/SignInForm"
import {
  history,
  render,
  waitForLoaderToBeRemoved,
} from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import { LoginPage } from "./LoginPage"
import * as TypesGen from "api/typesGenerated"
import { i18n } from "i18n"

const { t } = i18n

describe("LoginPage", () => {
  beforeEach(() => {
    history.replace("/login")
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }))
      }),
    )
  })

  it("renders the sign-in form", async () => {
    // When
    render(<LoginPage />)

    // Then
    await screen.findByText(Language.passwordSignIn)
  })

  it("shows an error message if SignIn fails", async () => {
    // Given
    const apiErrorMessage = "Something wrong happened"
    server.use(
      // Make login fail
      rest.post("/api/v2/users/login", async (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({ message: apiErrorMessage }))
      }),
    )

    // When
    render(<LoginPage />)
    await waitForLoaderToBeRemoved()
    const email = screen.getByLabelText(Language.emailLabel)
    const password = screen.getByLabelText(Language.passwordLabel)
    await userEvent.type(email, "test@coder.com")
    await userEvent.type(password, "password")
    // Click sign-in
    const signInButton = await screen.findByText(Language.passwordSignIn)
    fireEvent.click(signInButton)

    // Then
    const errorMessage = await screen.findByText(apiErrorMessage)
    expect(errorMessage).toBeDefined()
    expect(history.location.pathname).toEqual("/login")
  })

  it("shows github authentication when enabled", async () => {
    const authMethods: TypesGen.AuthMethods = {
      password: { enabled: true },
      github: { enabled: true },
      oidc: { enabled: true, signInText: "", iconUrl: "" },
    }

    // Given
    server.use(
      rest.get("/api/v2/users/authmethods", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(authMethods))
      }),
    )

    // When
    render(<LoginPage />)

    // Then
    expect(screen.queryByText(Language.passwordSignIn)).not.toBeInTheDocument()
    await screen.findByText(Language.githubSignIn)
  })

  it("redirects to the setup page if there is no first user", async () => {
    // Given
    server.use(
      rest.get("/api/v2/users/first", async (req, res, ctx) => {
        return res(ctx.status(404))
      }),
    )

    // When
    render(
      <Routes>
        <Route path="/login" element={<LoginPage />}></Route>
        <Route path="/setup" element={<h1>Setup</h1>}></Route>
      </Routes>,
    )

    // Then
    await screen.findByText("Setup")
  })

  it("hides password authentication if OIDC/GitHub is enabled and displays on click", async () => {
    const authMethods: TypesGen.AuthMethods = {
      password: { enabled: true },
      github: { enabled: true },
      oidc: { enabled: true, signInText: "", iconUrl: "" },
    }

    // Given
    server.use(
      rest.get("/api/v2/users/authmethods", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(authMethods))
      }),
    )

    // When
    render(<LoginPage />)

    // Then
    expect(screen.queryByText(Language.passwordSignIn)).not.toBeInTheDocument()
    await screen.findByText(Language.githubSignIn)

    const showPasswordLabel = t("showPassword", { ns: "loginPage" })
    const showPasswordAuthLink = screen.getByText(showPasswordLabel)
    await userEvent.click(showPasswordAuthLink)

    await screen.findByText(Language.passwordSignIn)
    await screen.findByText(Language.githubSignIn)
  })
})
