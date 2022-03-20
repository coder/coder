import axios from "axios"
import { getApiKey, login, logout } from "."
import { LoginResponse, APIKeyResponse } from "./types"

// Mock the axios module so that no real network requests are made, but rather
// we swap in a resolved or rejected value
//
// See: https://jestjs.io/docs/mock-functions#mocking-modules
jest.mock("axios")

describe("api.ts", () => {
  describe("login", () => {
    it("should return LoginResponse", async () => {
      // given
      const loginResponse: LoginResponse = {
        session_token: "abc_123_test",
      }
      const axiosMockPost = jest.fn().mockImplementationOnce(() => {
        return Promise.resolve({ data: loginResponse })
      })
      axios.post = axiosMockPost

      // when
      const result = await login("test", "123")

      // then
      expect(axiosMockPost).toHaveBeenCalled()
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
      const apiKeyResponse: APIKeyResponse = {
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
})
