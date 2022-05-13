import { screen, waitFor } from "@testing-library/react"
import React from "react"
import * as API from "../../api/api"
import { renderWithAuth } from "../../testHelpers/renderHelpers"
import { checks } from "../../xServices/auth/authXService"
import { Language as AdminDropdownLanguage } from "../AdminDropdown/AdminDropdown"
import { Navbar } from "./Navbar"

beforeEach(() => {
  jest.resetAllMocks()
})

describe("Navbar", () => {
  describe("when user has permission to read all users", () => {
    it("displays the admin menu", async () => {
      const checkUserPermissionsSpy = jest.spyOn(API, "checkUserPermissions").mockResolvedValueOnce({
        [checks.readAllUsers]: true,
      })

      renderWithAuth(<Navbar />)

      // Wait for the request is done
      await waitFor(() => expect(checkUserPermissionsSpy).toBeCalledTimes(1))
      await screen.findByRole("button", { name: AdminDropdownLanguage.menuTitle })
    })
  })

  describe("when user has NO permission to read all users", () => {
    it("does not display the admin menu", async () => {
      const checkUserPermissionsSpy = jest.spyOn(API, "checkUserPermissions").mockResolvedValueOnce({
        [checks.readAllUsers]: false,
      })
      renderWithAuth(<Navbar />)

      // Wait for the request is done
      await waitFor(() => expect(checkUserPermissionsSpy).toBeCalledTimes(1))
      expect(screen.queryByRole("button", { name: AdminDropdownLanguage.menuTitle })).not.toBeInTheDocument()
    })
  })
})
