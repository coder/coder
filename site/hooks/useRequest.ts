import { useState, useEffect } from "react"

export type RequestState<TPayload> =
  | {
      state: "loading"
    }
  | {
      state: "error"
      error: Error
    }
  | {
      state: "success"
      payload: TPayload
    }

export const useRequestor = <TPayload>(fn: () => Promise<TPayload>) => {
  const [requestState, setRequestState] = useState<RequestState<TPayload>>({ state: "loading" })

  useEffect(() => {
    const f = async () => {
      try {
        const response = await fn()
        setRequestState({ state: "success", payload: response })
      } catch (err) {
        setRequestState({ state: "error", error: err })
      }
    }
    f()
  })

  return requestState
}
