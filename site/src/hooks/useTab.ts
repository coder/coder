import { useSearchParams } from "react-router-dom"

export interface UseTabResult {
  value: string | null
  set: (value: string) => void
}

export const useTab = (tabKey: string): UseTabResult => {
  const [searchParams, setSearchParams] = useSearchParams()
  const value = searchParams.get(tabKey)

  return {
    value,
    set: (value: string) => {
      searchParams.set(tabKey, value)
      setSearchParams(searchParams)
    },
  }
}
