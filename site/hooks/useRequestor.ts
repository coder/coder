import { useState, useEffect } from "react"
import isReady from "next/router"

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

// TODO: Replace with `useSWR`
export const useRequestor = <TPayload>(fn: () => Promise<TPayload>, deps: any[] = []): RequestState<TPayload> => {
  const [requestState, setRequestState] = useState<RequestState<TPayload>>({ state: "loading" })

  useEffect(() => {
    // Initially, some parameters might not be available - make sure all query parameters are set
    // as a courtesy to users of this hook.
    if (!isReady) {
      return
    }

    let cancelled = false
    const f = async () => {
      try {
        const response = await fn()
        if (!cancelled) {
          setRequestState({ state: "success", payload: response })
        }
      } catch (err) {
        if (!cancelled) {
          setRequestState({ state: "error", error: err })
        }
      }
    }
    f()

    return () => {
      cancelled = true
    }
  }, [isReady, ...deps])

  return requestState
}
