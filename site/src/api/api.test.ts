import axios from "axios"
import { getApiKey, getURLWithSearchParams, login, logout } from "./api"
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

  describe("getURLWithSearchParams - workspaces", () => {
    it.each<[string, TypesGen.WorkspaceFilter | undefined, string]>([
      ["/api/v2/workspaces", undefined, "/api/v2/workspaces"],

      ["/api/v2/workspaces", { q: "" }, "/api/v2/workspaces"],
      ["/api/v2/workspaces", { q: "owner:1" }, "/api/v2/workspaces?q=owner%3A1"],

      ["/api/v2/workspaces", { q: "owner:me" }, "/api/v2/workspaces?q=owner%3Ame"],
    ])(`Workspaces - getURLWithSearchParams(%p, %p) returns %p`, (basePath, filter, expected) => {
      expect(getURLWithSearchParams(basePath, filter)).toBe(expected)
    })
  })

  describe("getURLWithSearchParams - users", () => {
    it.each<[string, TypesGen.UsersRequest | undefined, string]>([
      ["/api/v2/users", undefined, "/api/v2/users"],
      ["/api/v2/users", { q: "status:active" }, "/api/v2/users?q=status%3Aactive"],
      ["/api/v2/users", { q: "" }, "/api/v2/users"],
    ])(`Users - getURLWithSearchParams(%p, %p) returns %p`, (basePath, filter, expected) => {
      expect(getURLWithSearchParams(basePath, filter)).toBe(expected)
    })
  })
})
