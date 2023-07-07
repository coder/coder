import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react"

const LOCAL_PREFERENCES_KEY = "local-preferences"

const defaultValues = {
  buildLogsVisibility: "visible" as "visible" | "hide",
}

type LocalPreferencesValues = typeof defaultValues
type LocalPreference = keyof LocalPreferencesValues
type LocalPreferenceContextValues = {
  values: LocalPreferencesValues
  getPreference: (
    name: LocalPreference,
  ) => LocalPreferencesValues[LocalPreference]
  setPreference: (
    name: LocalPreference,
    value: LocalPreferencesValues[LocalPreference],
  ) => void
}

const LocalPreferencesContext = createContext<
  LocalPreferenceContextValues | undefined
>(undefined)

export const LocalPreferenceProvider = ({
  children,
}: {
  children: ReactNode
}) => {
  const [state, setState] = useState<{
    ready: boolean
    values: LocalPreferencesValues
  }>({ ready: false, values: defaultValues })

  useEffect(() => {
    const preferencesStr = window.localStorage.getItem(LOCAL_PREFERENCES_KEY)
    if (preferencesStr) {
      try {
        const values = JSON.parse(preferencesStr)
        setState({ ready: true, values })
        return
      } catch (error) {
        console.warn(
          "Error on parsing local preferences. Default values are used.",
        )
      }
    }

    setState((state) => ({ ...state, ready: true }))
  }, [])

  const getPreference: LocalPreferenceContextValues["getPreference"] =
    useCallback(
      (name) => {
        return state.values[name]
      },
      [state.values],
    )

  const setPreference: LocalPreferenceContextValues["setPreference"] =
    useCallback((name, value) => {
      setState((state) => {
        const newState = {
          ...state,
          values: {
            ...state.values,
            [name]: value,
          },
        }
        window.localStorage.setItem(
          LOCAL_PREFERENCES_KEY,
          JSON.stringify(newState),
        )
        return newState
      })
    }, [])

  return (
    <LocalPreferencesContext.Provider
      value={
        state.ready
          ? {
              values: state.values,
              getPreference,
              setPreference,
            }
          : undefined
      }
    >
      {children}
    </LocalPreferencesContext.Provider>
  )
}

export const useLocalPreferences = () => {
  const context = useContext(LocalPreferencesContext)
  if (context === undefined) {
    throw new Error(
      "useLocalPreference must be used within a LocalPreferenceProvider",
    )
  }
  return context
}
