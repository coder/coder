import axios from "axios"
import { getApiKey, getWorkspacesURL, login, logout } from "./api"
import * as TypesGen from "./typesGenerated"

describe("api.ts", () => {
  describe("login", () => {
    it("should return LoginResponse", async () => {
      // given
      const loginResponse: TypesGen.LoginWithPasswordResponse = {
        session_token: "abc_123_test",
      }
      jest.spyOn(axios, "post").mockResolvedValueOnce({ data: loginResponse })

      // when
      const result = await login("test", "123")

      // then
      expect(axios.post).toHaveBeenCalled()
      expect(result).toStrictEqual(loginResponse)
    })

    it("should throw an error on 401", async () => {
      // given
      // ..ensure that we await our expect assertion in async/await test
      expect.assertions(1)
      const expectedError = {
        message: "Validation failed",
        errors: [{ field: "email", code: "email" }],
      }
      const axiosMockPost = jest.fn().mockImplementationOnce(() => {
        return Promise.reject(expectedError)
      })
      axios.post = axiosMockPost

      try {
        await login("test", "123")
      } catch (error) {
        expect(error).toStrictEqual(expectedError)
      }
    })
  })

  describe("logout", () => {
    it("should return without erroring", async () => {
      // given
      const axiosMockPost = jest.fn().mockImplementationOnce(() => {
        return Promise.resolve()
      })
      axios.post = axiosMockPost

      // when
      await logout()

      // then
      expect(axiosMockPost).toHaveBeenCalled()
    })

    it("should throw an error on 500", async () => {
      // given
      // ..ensure that we await our expect assertion in async/await test
      expect.assertions(1)
      const expectedError = {
        message: "Failed to logout.",
      }
      const axiosMockPost = jest.fn().mockImplementationOnce(() => {
        return Promise.reject(expectedError)
      })
      axios.post = axiosMockPost

      try {
        await logout()
      } catch (error) {
        expect(error).toStrictEqual(expectedError)
      }
    })
  })

  describe("getApiKey", () => {
    it("should return APIKeyResponse", async () => {
      // given
      const apiKeyResponse: TypesGen.GenerateAPIKeyResponse = {
        key: "abc_123_test",
      }
      const axiosMockPost = jest.fn().mockImplementationOnce(() => {
        return Promise.resolve({ data: apiKeyResponse })
      })
      axios.post = axiosMockPost

      // when
      const result = await getApiKey()

      // then
      expect(axiosMockPost).toHaveBeenCalled()
      expect(result).toStrictEqual(apiKeyResponse)
    })

    it("should throw an error on 401", async () => {
      // given
      // ..ensure that we await our expect assertion in async/await test
      expect.assertions(1)
      const expectedError = {
        message: "No Cookie!",
      }
      const axiosMockPost = jest.fn().mockImplementationOnce(() => {
        return Promise.reject(expectedError)
      })
      axios.post = axiosMockPost

      try {
        await getApiKey()
      } catch (error) {
        expect(error).toStrictEqual(expectedError)
      }
    })
  })

  describe("getWorkspacesURL", () => {
    it.each<[TypesGen.WorkspaceFilter | undefined, string]>([
      [undefined, "/api/v2/workspaces"],

      [{ OrganizationID: "1", Owner: "" }, "/api/v2/workspaces?organization_id=1"],
      [{ OrganizationID: "", Owner: "1" }, "/api/v2/workspaces?owner=1"],

      [{ OrganizationID: "1", Owner: "me" }, "/api/v2/workspaces?organization_id=1&owner=me"],
    ])(`getWorkspacesURL(%p) returns %p`, (filter, expected) => {
      expect(getWorkspacesURL(filter)).toBe(expected)
    })
  })
})
